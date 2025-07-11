package chat

import (
	"context"
	"fmt"

	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// 새로운 채팅 서비스 인스턴스 생성
func NewChatService(kafkaBrokers []string) *ChatService {
	// 각 관리자 초기화
	users := NewUsers()
	rooms := NewRooms()
	streams := NewStreams()
	kafka := NewKafka(kafkaBrokers)
	
	// 메시지 관리자는 나중에 WebSocket 허브가 설정된 후 초기화
	messages := &Messages{
		producer: kafka.producer,
		wsHub:    nil,
	}
	
	service := &ChatService{
		users:    users,
		rooms:    rooms,
		messages: messages,
		streams:  streams,
		kafka:    kafka,
		wsHub:    nil,
	}
	
	return service
}

// WebSocket 허브 설정
func (s *ChatService) SetWebSocketHub(hub interface {
	BroadcastMessage(message *pb.ChatMessage)
	BroadcastRoomUpdate(room *pb.Room)
}) {
	s.wsHub = hub
	s.messages.wsHub = hub
}

// Kafka 컨슈머 시작
func (s *ChatService) StartKafkaConsumer(ctx context.Context) error {
	return s.kafka.StartKafkaConsumer(ctx)
}

// 사용자 등록 요청 처리
func (s *ChatService) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	return s.users.RegisterUser(ctx, req)
}

// 메시지 전송 요청 처리
func (s *ChatService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	// 사용자 존재 확인
	user, exists := s.users.GetUser(req.UserId)
	if !exists {
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	// 채팅방 존재 및 참여 여부 확인
	room, roomExists := s.rooms.GetRoom(req.RoomId)
	if !roomExists {
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "존재하지 않는 채팅방입니다",
		}, nil
	}
	
	// 사용자가 채팅방에 참여 중인지 확인
	if _, joined := room.Users[req.UserId]; !joined {
		return &pb.SendMessageResponse{
			Success: false,
			Error:   "채팅방에 참여하지 않은 사용자입니다",
		}, nil
	}
	
	return s.messages.SendMessage(ctx, req, user, s.streams)
}

// 메시지 스트림 구독 요청 처리
func (s *ChatService) SubscribeToMessages(req *pb.SubscribeRequest, stream pb.ChatService_SubscribeToMessagesServer) error {
	// 사용자 존재 확인
	_, exists := s.users.GetUser(req.UserId)
	if !exists {
		return fmt.Errorf("존재하지 않는 사용자입니다")
	}
	
	// 채팅방 존재 및 참여 여부 확인
	room, roomExists := s.rooms.GetRoom(req.RoomId)
	if !roomExists {
		return fmt.Errorf("존재하지 않는 채팅방입니다")
	}
	
	// 사용자가 채팅방에 참여 중인지 확인
	if _, joined := room.Users[req.UserId]; !joined {
		return fmt.Errorf("채팅방에 참여하지 않은 사용자입니다")
	}
	
	return s.streams.SubscribeToMessages(req, stream)
}

// 사용자 목록 조회 요청 처리
func (s *ChatService) GetUsers(ctx context.Context, req *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	return s.users.GetUsers(ctx, req)
}

// 채팅방 생성 요청 처리
func (s *ChatService) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.CreateRoomResponse, error) {
	// 사용자 존재 확인
	user, exists := s.users.GetUser(req.CreatorId)
	if !exists {
		return &pb.CreateRoomResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	return s.rooms.CreateRoom(ctx, req, user, s.wsHub, s.kafka)
}

// 채팅방 참여 요청 처리
func (s *ChatService) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	// 사용자 존재 확인
	user, userExists := s.users.GetUser(req.UserId)
	if !userExists {
		return &pb.JoinRoomResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	return s.rooms.JoinRoom(ctx, req, user, s.kafka)
}

// 채팅방 나가기 요청 처리
func (s *ChatService) LeaveRoom(ctx context.Context, req *pb.LeaveRoomRequest) (*pb.LeaveRoomResponse, error) {
	// 사용자 존재 확인
	user, userExists := s.users.GetUser(req.UserId)
	if !userExists {
		return &pb.LeaveRoomResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	return s.rooms.LeaveRoom(ctx, req, user, s.streams, s.kafka)
}

// 채팅방 목록 조회 요청 처리
func (s *ChatService) GetRooms(ctx context.Context, req *pb.GetRoomsRequest) (*pb.GetRoomsResponse, error) {
	return s.rooms.GetRooms(ctx, req)
}

// 사용자 상태 변경 요청 처리
func (s *ChatService) UpdateUserStatus(ctx context.Context, req *pb.UpdateUserStatusRequest) (*pb.UpdateUserStatusResponse, error) {
	response, err := s.users.UpdateUserStatus(ctx, req)
	if err != nil || !response.Success {
		return response, err
	}
	
	// 사용자 정보 가져오기
	user, _ := s.users.GetUser(req.UserId)
	
	// 상태 변경 알림 메시지 브로드캐스트
	statusMsg := "오프라인"
	if req.Online {
		statusMsg = "온라인"
	}
	
	// 사용자가 참여 중인 채팅방에만 상태 변경 알림
	rooms, _ := s.rooms.GetRooms(ctx, &pb.GetRoomsRequest{})
	for _, room := range rooms.Rooms {
		roomInfo, _ := s.rooms.GetRoom(room.RoomId)
		if roomInfo != nil {
			// 사용자가 참여 중인 방에만 알림
			if _, isMember := roomInfo.Users[req.UserId]; !isMember {
				continue
			}
			message := s.messages.CreateSystemMessage(room.RoomId, fmt.Sprintf("%s님이 %s 상태가 되었습니다", user.Username, statusMsg))
			s.streams.BroadcastMessage(room.RoomId, message)
		}
	}
	
	return response, nil
}

// 채팅방 삭제 요청 처리
func (s *ChatService) DeleteRoom(ctx context.Context, req *pb.DeleteRoomRequest) (*pb.DeleteRoomResponse, error) {
	return s.rooms.DeleteRoom(ctx, req, s.streams, s.kafka)
} 
