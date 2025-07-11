package chat

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// 채팅방 관리 생성
func NewRooms() *Rooms {
	r := &Rooms{
		rooms: make(map[string]*Room),
	}
	r.createDefaultRoom()
	return r
}

// 기본 채팅방 생성
func (r *Rooms) createDefaultRoom() {
	defaultRoomID := "general"
	defaultRoom := &Room{
		ID:        defaultRoomID,
		Name:      "일반",
		CreatorID: "system",
		Users:     make(map[string]*User),
		Created:   time.Now(),
	}
	
	r.roomsMux.Lock()
	r.rooms[defaultRoomID] = defaultRoom
	r.roomsMux.Unlock()
	
	log.Printf("기본 채팅방이 생성되었습니다: %s", defaultRoom.Name)
}

// 채팅방 생성
func (r *Rooms) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest, user *User, wsHub interface {
	BroadcastRoomUpdate(room *pb.Room)
}, kafka *Kafka) (*pb.CreateRoomResponse, error) {
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
	
	r.roomsMux.Lock()
	r.rooms[roomID] = room
	r.roomsMux.Unlock()
	
	// WebSocket으로 방 업데이트 브로드캐스트
	if wsHub != nil {
		pbRoom := &pb.Room{
			RoomId:    room.ID,
			Name:      room.Name,
			CreatorId: room.CreatorID,
			UserCount: int32(len(room.Users)),
			CreatedAt: room.Created.Unix(),
		}
		wsHub.BroadcastRoomUpdate(pbRoom)
	}
	
	// Kafka에 채팅방 생성 이벤트 발행
	if kafka != nil {
		go func() {
			data := map[string]interface{}{
				"room_name": req.Name,
			}
			if err := kafka.PublishRoomEvent(context.Background(), "room_created", roomID, req.CreatorId, data); err != nil {
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

// 채팅방 참여
func (r *Rooms) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest, user *User, kafka *Kafka) (*pb.JoinRoomResponse, error) {
	log.Printf("=== JoinRoom 요청 시작 ===")
	log.Printf("사용자 ID: %s, 방 ID: %s", req.UserId, req.RoomId)
	
	r.roomsMux.Lock()
	defer r.roomsMux.Unlock()
	
	room, roomExists := r.rooms[req.RoomId]
	if !roomExists {
		log.Printf("채팅방이 존재하지 않음: %s", req.RoomId)
		return &pb.JoinRoomResponse{
			Success: false,
			Error:   "존재하지 않는 채팅방입니다",
		}, nil
	}
	
	log.Printf("채팅방 확인됨: %s (%s)", room.Name, room.ID)
	log.Printf("현재 방 참여자: %v", r.GetUsernames(room.Users))
	
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
	log.Printf("추가 후 방 참여자: %v", r.GetUsernames(room.Users))
	
	// Kafka에 채팅방 참여 이벤트 발행
	if kafka != nil {
		go func() {
			data := map[string]interface{}{
				"username": user.Username,
			}
			if err := kafka.PublishRoomEvent(context.Background(), "user_joined", req.RoomId, req.UserId, data); err != nil {
				log.Printf("Kafka 이벤트 발행 실패: %v", err)
			}
		}()
	}
	
	log.Printf("=== JoinRoom 요청 성공 ===")
	return &pb.JoinRoomResponse{
		Success: true,
	}, nil
}

// 채팅방 나가기
func (r *Rooms) LeaveRoom(ctx context.Context, req *pb.LeaveRoomRequest, user *User, streams *Streams, kafka *Kafka) (*pb.LeaveRoomResponse, error) {
	log.Printf("=== LeaveRoom 요청 시작 ===")
	log.Printf("사용자 ID: %s, 방 ID: %s", req.UserId, req.RoomId)

	r.roomsMux.Lock()
	defer r.roomsMux.Unlock()
	
	room, roomExists := r.rooms[req.RoomId]
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
	if streams != nil {
		streams.RemoveUserFromRoom(req.RoomId, req.UserId)
	}
	
	// Kafka에 채팅방 나가기 이벤트 발행
	if kafka != nil {
		go func() {
			data := map[string]interface{}{
				"username": user.Username,
			}
			if err := kafka.PublishRoomEvent(context.Background(), "user_left", req.RoomId, req.UserId, data); err != nil {
				log.Printf("Kafka 이벤트 발행 실패: %v", err)
			}
		}()
	}
	
	log.Printf("사용자가 채팅방을 나갔습니다: %s -> %s (남은 사용자: %d)", user.Username, req.RoomId, len(room.Users))
	
	log.Printf("=== LeaveRoom 요청 성공 ===")
	return &pb.LeaveRoomResponse{
		Success: true,
	}, nil
}

// 채팅방 목록 조회
func (r *Rooms) GetRooms(ctx context.Context, req *pb.GetRoomsRequest) (*pb.GetRoomsResponse, error) {
	r.roomsMux.RLock()
	defer r.roomsMux.RUnlock()
	
	var rooms []*pb.Room
	for _, room := range r.rooms {
		userCount := int32(len(room.Users))
		log.Printf("채팅방 '%s' (ID: %s) - 참여자 수: %d, 참여자: %v", 
			room.Name, room.ID, userCount, r.GetUsernames(room.Users))
		
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

// 채팅방 삭제
func (r *Rooms) DeleteRoom(ctx context.Context, req *pb.DeleteRoomRequest, streams *Streams, kafka *Kafka) (*pb.DeleteRoomResponse, error) {
	r.roomsMux.Lock()

	// 기본방은 삭제 불가
	defaultRoomID := "general"
	if req.RoomId == defaultRoomID {
		r.roomsMux.Unlock()
		return &pb.DeleteRoomResponse{
			Success: false,
			Error:   "기본 채팅방은 삭제할 수 없습니다.",
		}, nil
	}

	room, exists := r.rooms[req.RoomId]
	if !exists {
		r.roomsMux.Unlock()
		return &pb.DeleteRoomResponse{
			Success: false,
			Error:   "존재하지 않는 채팅방입니다.",
		}, nil
	}

	// 생성자만 삭제 가능
	if room.CreatorID != req.UserId {
		r.roomsMux.Unlock()
		return &pb.DeleteRoomResponse{
			Success: false,
			Error:   "방 생성자만 삭제할 수 있습니다.",
		}, nil
	}

	// 채팅방에 사용자가 있는 경우 삭제 불가
	if len(room.Users) > 0 {
		r.roomsMux.Unlock()
		return &pb.DeleteRoomResponse{
			Success: false,
			Error:   "사용자가 있는 채팅방은 삭제할 수 없습니다.",
		}, nil
	}

	// 방 삭제
	delete(r.rooms, req.RoomId)

	// 관련 스트림도 정리
	if streams != nil {
		streams.RemoveRoom(req.RoomId)
	}

	// Kafka에 채팅방 삭제 이벤트 발행
	if kafka != nil {
		go func() {
			if err := kafka.PublishRoomEvent(context.Background(), "room_deleted", req.RoomId, req.UserId, nil); err != nil {
				log.Printf("Kafka 이벤트 발행 실패: %v", err)
			}
		}()
	}

	log.Printf("채팅방이 삭제되었습니다: %s (ID: %s)", room.Name, req.RoomId)

	return &pb.DeleteRoomResponse{
		Success: true,
	}, nil
}

// 채팅방 존재 확인
func (r *Rooms) GetRoom(roomID string) (*Room, bool) {
	r.roomsMux.RLock()
	defer r.roomsMux.RUnlock()
	
	room, exists := r.rooms[roomID]
	return room, exists
}

// 사용자 이름 목록 반환
func (r *Rooms) GetUsernames(users map[string]*User) []string {
	var usernames []string
	for _, user := range users {
		usernames = append(usernames, user.Username)
	}
	return usernames
} 
