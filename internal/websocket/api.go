package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// API 서버 구조체
type APIServer struct {
	grpcClient pb.ChatServiceClient
	hub        *Hub
}

// 새로운 API 서버 생성
func NewAPIServer(grpcAddr string, hub *Hub) (*APIServer, error) {
	// gRPC 클라이언트 연결
	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("gRPC 연결 실패: %v", err)
	}

	client := pb.NewChatServiceClient(conn)

	return &APIServer{
		grpcClient: client,
		hub:        hub,
	}, nil
}

// CORS
func (s *APIServer) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// 라우터 설정
func (s *APIServer) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// API 엔드포인트
	mux.HandleFunc("/api/register", s.corsMiddleware(s.handleRegister))
	mux.HandleFunc("/api/send-message", s.corsMiddleware(s.handleSendMessage))
	mux.HandleFunc("/api/rooms", s.corsMiddleware(s.handleGetRooms))
	mux.HandleFunc("/api/users", s.corsMiddleware(s.handleGetUsers))
	mux.HandleFunc("/api/create-room", s.corsMiddleware(s.handleCreateRoom))
	mux.HandleFunc("/api/join-room", s.corsMiddleware(s.handleJoinRoom))
	mux.HandleFunc("/api/leave-room", s.corsMiddleware(s.handleLeaveRoom))
	mux.HandleFunc("/api/update-status", s.corsMiddleware(s.handleUpdateStatus))
	mux.HandleFunc("/api/delete-room", s.corsMiddleware(s.handleDeleteRoom))

	// WebSocket 엔드포인트
	mux.HandleFunc("/ws", s.corsMiddleware(s.hub.HandleWebSocket))

	// 정적 파일 서빙 (웹 클라이언트)
	mux.HandleFunc("/", s.corsMiddleware(s.handleStatic))

	return mux
}

// 사용자 등록
func (s *APIServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.RegisterUser(context.Background(), &pb.RegisterUserRequest{
		Username: req.Username,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 메시지 전송
func (s *APIServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID  string `json:"userId"`
		RoomID  string `json:"roomId"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.SendMessage(context.Background(), &pb.SendMessageRequest{
		UserId:  req.UserID,
		RoomId:  req.RoomID,
		Message: req.Message,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 성공하면 WebSocket으로 브로드캐스트
	if resp.Success {
		// gRPC 응답에서 메시지 정보를 추출하여 WebSocket으로 전송
		// 실제로는 gRPC 서비스에서 WebSocket 허브를 참조해야 함
		log.Printf("메시지 전송 성공: %s", resp.MessageId)
	}

	json.NewEncoder(w).Encode(resp)
}

// 채팅방 목록 조회
func (s *APIServer) handleGetRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := s.grpcClient.GetRooms(context.Background(), &pb.GetRoomsRequest{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 사용자 목록 조회
func (s *APIServer) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := s.grpcClient.GetUsers(context.Background(), &pb.GetUsersRequest{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 채팅방 생성
func (s *APIServer) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CreatorID string `json:"creatorId"`
		Name      string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.CreateRoom(context.Background(), &pb.CreateRoomRequest{
		CreatorId: req.CreatorID,
		Name:      req.Name,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 채팅방 참여
func (s *APIServer) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"userId"`
		RoomID string `json:"roomId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.JoinRoom(context.Background(), &pb.JoinRoomRequest{
		UserId: req.UserID,
		RoomId: req.RoomID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 채팅방 나가기
func (s *APIServer) handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"userId"`
		RoomID string `json:"roomId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.LeaveRoom(context.Background(), &pb.LeaveRoomRequest{
		UserId: req.UserID,
		RoomId: req.RoomID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 사용자 상태 업데이트
func (s *APIServer) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"userId"`
		Online bool   `json:"online"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.UpdateUserStatus(context.Background(), &pb.UpdateUserStatusRequest{
		UserId: req.UserID,
		Online: req.Online,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 방 삭제
func (s *APIServer) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"userId"`
		RoomID string `json:"roomId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	resp, err := s.grpcClient.DeleteRoom(context.Background(), &pb.DeleteRoomRequest{
		UserId: req.UserID,
		RoomId: req.RoomID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 정적 파일 서빙
func (s *APIServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := "web" + r.URL.Path
	if r.URL.Path == "/" {
		path = "web/index.html"
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
} 
