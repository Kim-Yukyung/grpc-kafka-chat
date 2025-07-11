package chat

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
	"github.com/Kim-Yukyung/grpc-kafka-chat/internal/kafka"
)

// 메시지 관리 생성
func NewMessages(producer *kafka.KafkaProducer, wsHub interface {
	BroadcastMessage(message *pb.ChatMessage)
}) *Messages {
	return &Messages{
		producer: producer,
		wsHub:    wsHub,
	}
}

// 메시지 전송
func (m *Messages) SendMessage(ctx context.Context, req *pb.SendMessageRequest, user *User, streams *Streams) (*pb.SendMessageResponse, error) {
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
	
	// Kafka에 메시지 발행 (비동기)
	if m.producer != nil {
		go func() {
			if err := m.producer.PublishMessage(context.Background(), "chat-messages", message); err != nil {
				log.Printf("Kafka 메시지 발행 실패: %v", err)
			}
		}()
	}
	
	// 메시지를 해당 채팅방의 모든 구독자에게 전송
	if streams != nil {
		streams.BroadcastMessage(req.RoomId, message)
	}
	
	// WebSocket으로도 브로드캐스트
	if m.wsHub != nil {
		m.wsHub.BroadcastMessage(message)
	}
	
	return &pb.SendMessageResponse{
		Success:   true,
		MessageId: messageID,
	}, nil
}

// 시스템 메시지 생성
func (m *Messages) CreateSystemMessage(roomID string, message string) *pb.ChatMessage {
	return &pb.ChatMessage{
		MessageId: uuid.New().String(),
		UserId:    "system",
		Username:  "시스템",
		Message:   message,
		RoomId:    roomID,
		Timestamp: time.Now().Unix(),
	}
} 
