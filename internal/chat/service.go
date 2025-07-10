package chat

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
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
	ID      string
	Name    string
	Users   map[string]*User
	Created time.Time
}

// 새로운 채팅 서비스 인스턴스 생성
func NewChatService() *ChatService {
	return &ChatService{
		users:   make(map[string]*User),
		streams: make(map[string]map[string]chan *pb.ChatMessage),
		rooms:   make(map[string]*Room),
	}
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
	
	// 메시지를 해당 채팅방의 모든 구독자에게 전송
	s.broadcastMessage(req.RoomId, message)
	
	return &pb.SendMessageResponse{
		Success:   true,
		MessageId: messageID,
	}, nil
}

// 메시지 스트림 구독 요청 처리
func (s *ChatService) SubscribeToMessages(req *pb.SubscribeRequest, stream pb.ChatService_SubscribeToMessagesServer) error {
	// 사용자 존재 확인
	s.usersMux.RLock()
	_, exists := s.users[req.UserId]
	s.usersMux.RUnlock()
	
	if !exists {
		return fmt.Errorf("존재하지 않는 사용자입니다")
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
		for _, messageChan := range streams {
			select {
			case messageChan <- message:
			default:
				log.Println("채널이 가득 차서 메시지를 드롭했습니다")
			}
		}
	}
} 
