package chat

import (
	"log"

	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// 스트리밍 관리 생성
func NewStreams() *Streams {
	return &Streams{
		streams: make(map[string]map[string]chan *pb.ChatMessage),
	}
}

// 메시지 스트림 구독
func (s *Streams) SubscribeToMessages(req *pb.SubscribeRequest, stream pb.ChatService_SubscribeToMessagesServer) error {
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

// 메시지 브로드캐스트
func (s *Streams) BroadcastMessage(roomID string, message *pb.ChatMessage) {
	s.streamsMux.RLock()
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

// 사용자를 방에서 제거
func (s *Streams) RemoveUserFromRoom(roomID string, userID string) {
	s.streamsMux.Lock()
	defer s.streamsMux.Unlock()
	
	if streams, exists := s.streams[roomID]; exists {
		delete(streams, userID)
	}
}

// 방 제거
func (s *Streams) RemoveRoom(roomID string) {
	s.streamsMux.Lock()
	defer s.streamsMux.Unlock()
	
	delete(s.streams, roomID)
} 
