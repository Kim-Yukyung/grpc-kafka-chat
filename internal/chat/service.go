package chat

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
	"github.com/Kim-Yukyung/grpc-kafka-chat/internal/kafka"
)

// 실시간 채팅 서비스 구현체
// gRPC 채팅 서버의 상태를 관리하는 구조체
type ChatService struct {
	pb.UnimplementedChatServiceServer // gPRC 인터페이스 기본 구현 제공
	
	// 전체 사용자 목록
	// user_id -> user_info
	users    map[string]*User
	usersMux sync.RWMutex // 동시성 접근 제어를 위한 mutex
	
	// gRPC 스트리밍 채널
	// room_id -> user_id -> message_chan
	// 채팅방에 참여한 사용자의 메시지 채널 관리
	streams    map[string]map[string]chan *pb.ChatMessage
	streamsMux sync.RWMutex
	
	// 채팅방 목록
	// room_id -> room_info
	rooms    map[string]*Room
	roomsMux sync.RWMutex
	
	// Kafka 프로듀서
	producer *kafka.KafkaProducer
	
	// Kafka 컨슈머
	consumer *kafka.KafkaConsumer
	
	// Kafka 설정
	kafkaBrokers []string
	messageTopic  string
	eventTopic    string
	
	// WebSocket 설정
	wsHub interface {
		BroadcastMessage(message *pb.ChatMessage)
		BroadcastRoomUpdate(room *pb.Room)
	}
}

// 사용자 정보 구조체
type User struct {
	ID       string
	Username string
	Online   bool
	JoinedAt time.Time
}

// 채팅방 정보 구조체
type Room struct {
	ID        string
	Name      string
	CreatorID string
	Users     map[string]*User
	Created   time.Time
}

// 새로운 채팅 서비스 인스턴스 생성
func NewChatService(kafkaBrokers []string) *ChatService {
	// ChatService 구조체 초기화
	service := &ChatService{
		users:         make(map[string]*User),
		streams:       make(map[string]map[string]chan *pb.ChatMessage),
		rooms:         make(map[string]*Room),
		kafkaBrokers:  kafkaBrokers,
		messageTopic:  "chat-messages",
		eventTopic:    "chat-events",
	}
	
	// Kafka가 설정된 경우 Kafka 프로듀서 초기화
	if len(kafkaBrokers) > 0 {
		service.producer = kafka.NewKafkaProducer(kafkaBrokers)
		log.Printf("Kafka 프로듀서가 초기화되었습니다. 브로커: %v", kafkaBrokers)
	} else {
		log.Printf("Kafka 브로커가 설정되지 않아 Kafka 기능을 비활성화합니다.")
	}
	
	// 기본 채팅방 생성
	service.createDefaultRoom()
	
	return service
}

// 기본 채팅방 생성
func (s *ChatService) createDefaultRoom() {
	defaultRoomID := "general"
	defaultRoom := &Room{
		ID:        defaultRoomID,
		Name:      "일반",
		CreatorID: "system",
		Users:     make(map[string]*User),
		Created:   time.Now(),
	}
	
	s.roomsMux.Lock()
	s.rooms[defaultRoomID] = defaultRoom
	s.roomsMux.Unlock()
	
	log.Printf("기본 채팅방이 생성되었습니다: %s", defaultRoom.Name)
}

// WebSocket 허브 설정
func (s *ChatService) SetWebSocketHub(hub interface {
	BroadcastMessage(message *pb.ChatMessage)
	BroadcastRoomUpdate(room *pb.Room)
}) {
	s.wsHub = hub
}

// Kafka 컨슈머 시작
func (s *ChatService) StartKafkaConsumer(ctx context.Context) error {
	if len(s.kafkaBrokers) == 0 {
		log.Println("Kafka 브로커가 설정되지 않아 컨슈머를 시작하지 않습니다.")
		return nil
	}
	
	// 메시지 컨슈머 시작
	messageConsumer := kafka.NewKafkaConsumer(s.kafkaBrokers, s.messageTopic, "chat-service-group")
	
	// 고루틴으로 메시지 컨슈머 시작
	go func() {
		if err := messageConsumer.ConsumeMessages(ctx, s.handleKafkaMessage); err != nil {
			log.Printf("메시지 컨슈머 오류: %v", err)
		}
	}()
	
	// 이벤트 컨슈머 시작
	eventConsumer := kafka.NewKafkaConsumer(s.kafkaBrokers, s.eventTopic, "chat-service-events-group")
	
	// 고루틴으로 이벤트 컨슈머 시작
	go func() {
		if err := eventConsumer.ConsumeEvents(ctx, s.handleKafkaEvent); err != nil {
			log.Printf("이벤트 컨슈머 오류: %v", err)
		}
	}()
	
	log.Printf("Kafka 컨슈머가 시작되었습니다. 메시지 토픽: %s, 이벤트 토픽: %s", s.messageTopic, s.eventTopic)
	return nil
}

// Kafka에서 받은 메시지를 처리
func (s *ChatService) handleKafkaMessage(message *pb.ChatMessage) error {
	log.Printf("Kafka에서 메시지 수신: %s", message.MessageId)
	
	// Kafka에서 받은 메시지는 다시 브로드캐스트하지 않음
	// (이미 gRPC로 전송되었으므로 중복 방지)

	return nil
}

// Kafka에서 받은 이벤트를 처리
func (s *ChatService) handleKafkaEvent(eventType string, roomID string, userID string, data map[string]interface{}) error {
	log.Printf("Kafka에서 이벤트 수신: %s (방: %s, 사용자: %s)", eventType, roomID, userID)
	
	// 이벤트 타입에 따른 처리
	switch eventType {
	case "room_created":
		log.Printf("채팅방 생성 이벤트: %s", roomID)
	case "user_joined":
		log.Printf("사용자 참여 이벤트: %s -> %s", userID, roomID)
	case "user_left":
		log.Printf("사용자 나가기 이벤트: %s -> %s", userID, roomID)
	case "room_deleted":
		log.Printf("채팅방 삭제 이벤트: %s", roomID)
	default:
		log.Printf("알 수 없는 이벤트 타입: %s", eventType)
	}
	
	return nil
}

// 사용자 등록 요청 처리
func (s *ChatService) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	// 사용자 목록 동시성 접근 제어
	s.usersMux.Lock()
	defer s.usersMux.Unlock() // 함수 종료 시 자동으로 락 해제

	// 사용자 이름 중복 체크
	for _, user := range s.users {
		if user.Username == req.Username {
			return &pb.RegisterUserResponse{
				Success: false,
				Error:   "이미 존재하는 사용자 이름입니다",
			}, nil
		}
	}
	
	// 새 사용자 생성
	userID := uuid.New().String()
	user := &User{
		ID:       userID,
		Username: req.Username,
		Online:   true,
		JoinedAt: time.Now(),
	}
	
	s.users[userID] = user
	
	return &pb.RegisterUserResponse{
		Success: true,
		UserId:  userID,
	}, nil
}

// 메시지 전송 요청 처리
func (s *ChatService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	// 사용자 존재 확인
	s.usersMux.RLock() // 사용자 목록 읽기 락 획득
	user, exists := s.users[req.UserId]
	s.usersMux.RUnlock() // 사용자 목록 읽기 락 해제
	
	// 사용자 없을 경우 에러 반환
	if !exists {
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	// 채팅방 존재 및 참여 여부 확인
	s.roomsMux.RLock()
	room, roomExists := s.rooms[req.RoomId]
	if !roomExists {
		s.roomsMux.RUnlock()
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "존재하지 않는 채팅방입니다",
		}, nil
	}
	
	// 사용자가 채팅방에 참여 중인지 확인
	if _, joined := room.Users[req.UserId]; !joined {
		s.roomsMux.RUnlock()
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "채팅방에 참여하지 않은 사용자입니다",
		}, nil
	}
	
	// 메시지 생성
	messageID := uuid.New().String()
	message := &pb.ChatMessage{
		MessageId: messageID,
		UserId:    req.UserId,
		Username:  user.Username,
		Message:   req.Message,
		RoomId:    req.RoomId,
		Timestamp: time.Now().Unix(),
	}
	s.roomsMux.RUnlock()
	
	// Kafka에 메시지 발행 (비동기)
	if s.producer != nil {
		go func() {
			if err := s.producer.PublishMessage(context.Background(), s.messageTopic, message); err != nil {
				log.Printf("Kafka 메시지 발행 실패: %v", err)
			}
		}()
	}
	
	// 메시지를 해당 채팅방의 모든 구독자에게 전송
	s.broadcastMessage(req.RoomId, message)
	
	// WebSocket으로도 브로드캐스트
	if s.wsHub != nil {
		s.wsHub.BroadcastMessage(message)
	}
	
	return &pb.SendMessageResponse{
		Success:   true,
		MessageId: messageID,
	}, nil
}

// 메시지 스트림 구독 요청 처리
// 특정 채팅방에서 실시간 메시지 수신을 위한 구독 채널 생성 및 메시지 스트리밍
func (s *ChatService) SubscribeToMessages(req *pb.SubscribeRequest, stream pb.ChatService_SubscribeToMessagesServer) error {
	// 사용자 존재 확인
	s.usersMux.RLock()
	_, exists := s.users[req.UserId]
	s.usersMux.RUnlock()
	
	if !exists {
		return fmt.Errorf("존재하지 않는 사용자입니다")
	}
	
	// 채팅방 존재 및 참여 여부 확인
	s.roomsMux.RLock()
	room, roomExists := s.rooms[req.RoomId]
	if !roomExists {
		s.roomsMux.RUnlock()
		return fmt.Errorf("존재하지 않는 채팅방입니다")
	}
	
	// 사용자가 채팅방에 참여 중인지 확인
	if _, joined := room.Users[req.UserId]; !joined {
		s.roomsMux.RUnlock()
		return fmt.Errorf("채팅방에 참여하지 않은 사용자입니다")
	}
	
	// 스트림 채널 생성
	messageChan := make(chan *pb.ChatMessage, 100)
	
	// 스트림 등록
	s.streamsMux.Lock()
	if s.streams[req.RoomId] == nil {
		s.streams[req.RoomId] = make(map[string]chan *pb.ChatMessage)
	}
	s.streams[req.RoomId][req.UserId] = messageChan
	s.streamsMux.Unlock()
	s.roomsMux.RUnlock()
	
	// 연결 해제 시 정리
	defer func() {
		s.streamsMux.Lock()
		if s.streams[req.RoomId] != nil {
			delete(s.streams[req.RoomId], req.UserId)
		}
		s.streamsMux.Unlock()
		close(messageChan)
	}()
	
	// 메시지 수신 및 전송
	for {
		select {
		case message := <-messageChan:
			if err := stream.Send(message); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// 사용자 목록 조회 요청 처리
func (s *ChatService) GetUsers(ctx context.Context, req *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	s.usersMux.RLock()
	defer s.usersMux.RUnlock()
	
	var users []*pb.User
	for _, user := range s.users {
		users = append(users, &pb.User{
			UserId:   user.ID,
			Username: user.Username,
			Online:   user.Online,
		})
	}
	
	return &pb.GetUsersResponse{
		Users: users,
	}, nil
}

// 메시지를 채팅방의 모든 구독자에게 전송 처리
func (s *ChatService) broadcastMessage(roomID string, message *pb.ChatMessage) {
	s.streamsMux.RLock() // 읽기만 할 때는 RLock 사용 -> 다중 읽기 허용 
	defer s.streamsMux.RUnlock()
	
	if streams, exists := s.streams[roomID]; exists {
		// 채팅방에 참여한 모든 사용자에게 메시지 전송
		userCount := len(streams)
		log.Printf("메시지 브로드캐스트 - 방ID: %s, 구독자 수: %d, 메시지: %s", roomID, userCount, message.Message)
		
		for userID, messageChan := range streams {
			select {
			case messageChan <- message:
				log.Printf("메시지 전송 성공 - 사용자: %s", userID)
			default:
				log.Printf("채널이 가득 차서 메시지를 드롭했습니다 - 사용자: %s", userID)
			}
		}
	} else {
		log.Printf("채팅방 %s에 구독자가 없습니다", roomID)
	}
}

// 채팅방 생성 요청 처리
func (s *ChatService) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.CreateRoomResponse, error) {
	// 사용자 존재 확인
	s.usersMux.RLock()
	user, exists := s.users[req.CreatorId]
	s.usersMux.RUnlock()
	
	if !exists {
		return &pb.CreateRoomResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	// 채팅방 생성
	roomID := uuid.New().String()
	room := &Room{
		ID:        roomID,
		Name:      req.Name,
		CreatorID: req.CreatorId,
		Users:     make(map[string]*User),
		Created:   time.Now(),
	}
	
	// 생성자를 채팅방에 자동으로 추가
	room.Users[req.CreatorId] = user
	
	s.roomsMux.Lock()
	s.rooms[roomID] = room
	s.roomsMux.Unlock()
	
	// WebSocket으로 방 업데이트 브로드캐스트
	if s.wsHub != nil {
		pbRoom := &pb.Room{
			RoomId:    room.ID,
			Name:      room.Name,
			CreatorId: room.CreatorID,
			UserCount: int32(len(room.Users)),
			CreatedAt: room.Created.Unix(),
		}
		s.wsHub.(interface {
			BroadcastRoomUpdate(room *pb.Room)
		}).BroadcastRoomUpdate(pbRoom)
	}
	
	// Kafka에 채팅방 생성 이벤트 발행
	if s.producer != nil {
		go func() {
			data := map[string]interface{}{
				"room_name": req.Name,
			}
			if err := s.producer.PublishRoomEvent(context.Background(), s.eventTopic, "room_created", roomID, req.CreatorId, data); err != nil {
				log.Printf("Kafka 이벤트 발행 실패: %v", err)
			}
		}()
	}
	
	log.Printf("새 채팅방이 생성되었습니다: %s (ID: %s, 생성자: %s)", req.Name, roomID, user.Username)
	
	return &pb.CreateRoomResponse{
		Success: true,
		RoomId:  roomID,
	}, nil
}

// 채팅방 참여 요청 처리
func (s *ChatService) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	log.Printf("=== JoinRoom 요청 시작 ===")
	log.Printf("사용자 ID: %s, 방 ID: %s", req.UserId, req.RoomId)
	
	// 사용자 존재 확인
	s.usersMux.RLock()
	user, userExists := s.users[req.UserId]
	s.usersMux.RUnlock()
	
	if !userExists {
		log.Printf("사용자가 존재하지 않음: %s", req.UserId)
		return &pb.JoinRoomResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	log.Printf("사용자 확인됨: %s (%s)", user.Username, user.ID)
	
	// 채팅방 존재 확인
	s.roomsMux.Lock()
	defer s.roomsMux.Unlock()
	
	room, roomExists := s.rooms[req.RoomId]
	if !roomExists {
		log.Printf("채팅방이 존재하지 않음: %s", req.RoomId)
		return &pb.JoinRoomResponse{
			Success: false,
			Error:   "존재하지 않는 채팅방입니다",
		}, nil
	}
	
	log.Printf("채팅방 확인됨: %s (%s)", room.Name, room.ID)
	log.Printf("현재 방 참여자: %v", getUsernames(room.Users))
	
	// 이미 참여 중인지 확인 - 이미 참여 중이어도 성공으로 처리
	if _, alreadyJoined := room.Users[req.UserId]; alreadyJoined {
		log.Printf("이미 참여 중인 사용자: %s (성공으로 처리)", user.Username)
		return &pb.JoinRoomResponse{
			Success: true,
		}, nil
	}
	
	// 채팅방에 사용자 추가
	room.Users[req.UserId] = user
	log.Printf("사용자 추가됨: %s -> %s", user.Username, room.Name)
	log.Printf("추가 후 방 참여자: %v", getUsernames(room.Users))
	
	// Kafka에 채팅방 참여 이벤트 발행
	if s.producer != nil {
		go func() {
			data := map[string]interface{}{
				"username": user.Username,
			}
			if err := s.producer.PublishRoomEvent(context.Background(), s.eventTopic, "user_joined", req.RoomId, req.UserId, data); err != nil {
				log.Printf("Kafka 이벤트 발행 실패: %v", err)
			}
		}()
	}
	
	log.Printf("=== JoinRoom 요청 성공 ===")
	return &pb.JoinRoomResponse{
		Success: true,
	}, nil
}

// 채팅방 나가기 요청 처리
func (s *ChatService) LeaveRoom(ctx context.Context, req *pb.LeaveRoomRequest) (*pb.LeaveRoomResponse, error) {
	log.Printf("=== LeaveRoom 요청 시작 ===")
	log.Printf("사용자 ID: %s, 방 ID: %s", req.UserId, req.RoomId)
	
	// user 정보 가져오기
	s.usersMux.RLock()
	user, userExists := s.users[req.UserId]
	s.usersMux.RUnlock()
	if !userExists {
		log.Printf("사용자가 존재하지 않음: %s", req.UserId)
		return &pb.LeaveRoomResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}

	s.roomsMux.Lock()
	defer s.roomsMux.Unlock()
	
	room, roomExists := s.rooms[req.RoomId]
	if !roomExists {
		log.Printf("채팅방이 존재하지 않음: %s", req.RoomId)
		return &pb.LeaveRoomResponse{
			Success: false,
			Error:   "존재하지 않는 채팅방입니다",
		}, nil
	}
	
	// 사용자가 채팅방에 참여 중인지 확인 - 참여하지 않아도 성공으로 처리
	if _, joined := room.Users[req.UserId]; !joined {
		log.Printf("참여하지 않은 채팅방이지만 성공으로 처리: %s", user.Username)
		return &pb.LeaveRoomResponse{
			Success: true,
		}, nil
	}
	
	// 채팅방에서 사용자 제거
	delete(room.Users, req.UserId)
	log.Printf("사용자 제거됨: %s -> %s", user.Username, room.Name)
	
	// 스트림에서도 제거
	s.streamsMux.Lock()
	if streams, exists := s.streams[req.RoomId]; exists {
		delete(streams, req.UserId)
	}
	s.streamsMux.Unlock()
	
	// Kafka에 채팅방 나가기 이벤트 발행
	if s.producer != nil {
		go func() {
			data := map[string]interface{}{
				"username": user.Username,
			}
			if err := s.producer.PublishRoomEvent(context.Background(), s.eventTopic, "user_left", req.RoomId, req.UserId, data); err != nil {
				log.Printf("Kafka 이벤트 발행 실패: %v", err)
			}
		}()
	}
	
	// 채팅방이 비어있고 기본 방이 아닌 경우에만 삭제
	// if len(room.Users) == 0 && req.RoomId != "general" {
	//     delete(s.rooms, req.RoomId)
	//     // 관련 스트림도 정리
	//     s.streamsMux.Lock()
	//     delete(s.streams, req.RoomId)
	//     s.streamsMux.Unlock()
	//     // Kafka에 채팅방 삭제 이벤트 발행
	//     if s.producer != nil {
	//         go func() {
	//             if err := s.producer.PublishRoomEvent(context.Background(), s.eventTopic, "room_deleted", req.RoomId, req.UserId, nil); err != nil {
	//                 log.Printf("Kafka 이벤트 발행 실패: %v", err)
	//             }
	//         }()
	//     }
	//     log.Printf("채팅방이 삭제되었습니다: %s", req.RoomId)
	// } else {
		log.Printf("사용자가 채팅방을 나갔습니다: %s -> %s (남은 사용자: %d)", user.Username, req.RoomId, len(room.Users))
	// }
	
	log.Printf("=== LeaveRoom 요청 성공 ===")
	return &pb.LeaveRoomResponse{
		Success: true,
	}, nil
}

// 채팅방 목록 조회 요청 처리
func (s *ChatService) GetRooms(ctx context.Context, req *pb.GetRoomsRequest) (*pb.GetRoomsResponse, error) {
	s.roomsMux.RLock()
	defer s.roomsMux.RUnlock()
	
	var rooms []*pb.Room
	for _, room := range s.rooms {
		userCount := int32(len(room.Users))
		log.Printf("채팅방 '%s' (ID: %s) - 참여자 수: %d, 참여자: %v", 
			room.Name, room.ID, userCount, getUsernames(room.Users))
		
		rooms = append(rooms, &pb.Room{
			RoomId:    room.ID,
			Name:      room.Name,
			CreatorId: room.CreatorID,
			UserCount: userCount,
			CreatedAt: room.Created.Unix(),
		})
	}
	
	return &pb.GetRoomsResponse{
		Rooms: rooms,
	}, nil
}

// 채팅방의 사용자 이름 목록을 반환
func getUsernames(users map[string]*User) []string {
	var usernames []string
	for _, user := range users {
		usernames = append(usernames, user.Username)
	}
	return usernames
}

// 사용자 상태 변경 요청 처리
func (s *ChatService) UpdateUserStatus(ctx context.Context, req *pb.UpdateUserStatusRequest) (*pb.UpdateUserStatusResponse, error) {
	s.usersMux.Lock()
	defer s.usersMux.Unlock()
	
	user, exists := s.users[req.UserId]
	if !exists {
		return &pb.UpdateUserStatusResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	user.Online = req.Online
	
	// 상태 변경 알림 메시지 브로드캐스트
	statusMsg := "오프라인"
	if req.Online {
		statusMsg = "온라인"
	}
	
	// 사용자가 참여 중인 채팅방에만 상태 변경 알림
	s.roomsMux.RLock()
	for roomID, room := range s.rooms {
		// 사용자가 참여 중인 방에만 알림
		if _, isMember := room.Users[req.UserId]; !isMember {
			continue
		}
		message := &pb.ChatMessage{
			MessageId: uuid.New().String(),
			UserId:    "system",
			Username:  "시스템",
			Message:   fmt.Sprintf("%s님이 %s 상태가 되었습니다", user.Username, statusMsg),
			RoomId:    roomID,
			Timestamp: time.Now().Unix(),
		}
		s.broadcastMessage(roomID, message)
	}
	s.roomsMux.RUnlock()
	
	return &pb.UpdateUserStatusResponse{
		Success: true,
	}, nil
} 

// 채팅방 삭제 요청 처리
func (s *ChatService) DeleteRoom(ctx context.Context, req *pb.DeleteRoomRequest) (*pb.DeleteRoomResponse, error) {
    s.roomsMux.Lock()

    // 기본방은 삭제 불가
    defaultRoomID := "general"
    if req.RoomId == defaultRoomID {
		s.roomsMux.Unlock()
        return &pb.DeleteRoomResponse{
            Success: false,
            Error:   "기본 채팅방은 삭제할 수 없습니다.",
        }, nil
    }

    room, exists := s.rooms[req.RoomId]
    if !exists {
		s.roomsMux.Unlock()
        return &pb.DeleteRoomResponse{
            Success: false,
            Error:   "존재하지 않는 채팅방입니다.",
        }, nil
    }

    // 생성자만 삭제 가능
    if room.CreatorID != req.UserId {
		s.roomsMux.Unlock()
        return &pb.DeleteRoomResponse{
            Success: false,
            Error:   "방 생성자만 삭제할 수 있습니다.",
        }, nil
    }

	// 채팅방에 사용자가 있는 경우 삭제 불가
    if len(room.Users) > 0 {
        s.roomsMux.Unlock()
        return &pb.DeleteRoomResponse{
            Success: false,
            Error:   "사용자가 있는 채팅방은 삭제할 수 없습니다.",
        }, nil
	}

    // 방 삭제
    delete(s.rooms, req.RoomId)
	s.roomsMux.Unlock()

    // 관련 스트림도 정리
    s.streamsMux.Lock()
    delete(s.streams, req.RoomId)
    s.streamsMux.Unlock()

    // Kafka에 채팅방 삭제 이벤트 발행
    if s.producer != nil {
        go func() {
            if err := s.producer.PublishRoomEvent(context.Background(), s.eventTopic, "room_deleted", req.RoomId, req.UserId, nil); err != nil {
                log.Printf("Kafka 이벤트 발행 실패: %v", err)
            }
        }()
    }

    log.Printf("채팅방이 삭제되었습니다: %s (ID: %s)", room.Name, req.RoomId)

    return &pb.DeleteRoomResponse{
        Success: true,
    }, nil
} 
