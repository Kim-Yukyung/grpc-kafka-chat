package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// WebSocket 연결을 위한 업그레이더
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocket 클라이언트 연결 정보
type Client struct {
	ID       string
	UserID   string
	Username string
	RoomID   string
	Conn     *websocket.Conn
	Send     chan []byte
	Hub      *Hub
}

// WebSocket 허브
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan *pb.ChatMessage
	register   chan *Client
	unregister chan *Client
	roomUpdate chan *pb.Room
	mutex      sync.RWMutex
}

// 새로운 허브 생성
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan *pb.ChatMessage, 100),
		register:   make(chan *Client, 100),
		unregister: make(chan *Client, 100),
		roomUpdate: make(chan *pb.Room, 100),
	}
}

// 허브 실행
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Printf("클라이언트 등록: %s (사용자: %s, 방: %s)", client.ID, client.Username, client.RoomID)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mutex.Unlock()
			log.Printf("클라이언트 해제: %s (사용자: %s, 방: %s)", client.ID, client.Username, client.RoomID)

		case message := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				// 같은 방의 클라이언트에게만 메시지 전송
				if client.RoomID == message.RoomId {
					select {
					case client.Send <- h.messageToJSON(message):
					default:
						close(client.Send)
						delete(h.clients, client)
					}
				}
			}
			h.mutex.RUnlock()

		case room := <-h.roomUpdate:
			// 모든 클라이언트에게 방 목록 업데이트 알림
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.Send <- h.roomUpdateToJSON(room):
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// 메시지를 JSON으로 변환
func (h *Hub) messageToJSON(message *pb.ChatMessage) []byte {
	data := map[string]interface{}{
		"type":      "message",
		"messageId": message.MessageId,
		"userId":    message.UserId,
		"username":  message.Username,
		"message":   message.Message,
		"roomId":    message.RoomId,
		"timestamp": message.Timestamp,
	}
	
	jsonData, _ := json.Marshal(data)
	return jsonData
}

// 방 업데이트를 JSON으로 변환
func (h *Hub) roomUpdateToJSON(room *pb.Room) []byte {
	data := map[string]interface{}{
		"type": "room_update",
		"room": map[string]interface{}{
			"room_id":    room.RoomId,
			"name":       room.Name,
			"creator_id": room.CreatorId,
			"user_count": room.UserCount,
			"created_at": room.CreatedAt,
		},
	}
	
	jsonData, _ := json.Marshal(data)
	return jsonData
}

// WebSocket 핸들러
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// URL 파라미터에서 사용자 정보 추출
	userID := r.URL.Query().Get("userId")
	username := r.URL.Query().Get("username")
	roomID := r.URL.Query().Get("roomId")

	if userID == "" || username == "" || roomID == "" {
		http.Error(w, "필수 파라미터가 누락되었습니다", http.StatusBadRequest)
		return
	}

	// WebSocket 연결 업그레이드
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket 업그레이드 실패: %v", err)
		return
	}

	// 클라이언트 생성
	client := &Client{
		ID:       fmt.Sprintf("%s-%d", userID, time.Now().UnixNano()),
		UserID:   userID,
		Username: username,
		RoomID:   roomID,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		Hub:      h,
	}

	// 허브에 클라이언트 등록
	h.register <- client

	// 클라이언트 고루틴 시작
	go client.writePump()
	go client.readPump()
}

// 클라이언트 메시지 읽기
func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket 읽기 오류: %v", err)
			}
			break
		}

		// 메시지 처리
		log.Printf("클라이언트 %s로부터 메시지 수신: %s", c.Username, string(message))
	}
}

// 클라이언트 메시지 쓰기
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// 메시지 브로드캐스트
func (h *Hub) BroadcastMessage(message *pb.ChatMessage) {
	h.broadcast <- message
}

// 방 업데이트 브로드캐스트
func (h *Hub) BroadcastRoomUpdate(room *pb.Room) {
	h.roomUpdate <- room
} 
