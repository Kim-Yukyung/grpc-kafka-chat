package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// Kafka 메시지를 발행하는 프로듀서 구조체
type KafkaProducer struct {
	writer *kafka.Writer
}

// Kafka 메시지를 발행하는 새로운 프로듀서 인스턴스 생성
func NewKafkaProducer(brokers []string) *KafkaProducer {
	writer := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Balancer: &kafka.LeastBytes{},
		Logger:   kafka.LoggerFunc(log.Printf),
	}

	return &KafkaProducer{
		writer: writer,
	}
}

// 채팅 메시지를 Kafka 토픽에 발행
func (p *KafkaProducer) PublishMessage(ctx context.Context, topic string, message *pb.ChatMessage) error {
	// 메시지를 JSON으로 직렬화
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("메시지 직렬화 실패: %v", err)
	}

	// Kafka에 메시지 발행
	err = p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(message.RoomId), // 방 ID를 키로 사용
		Value: messageBytes,
		Time:  time.Unix(message.Timestamp, 0),
	})

	if err != nil {
		return fmt.Errorf("Kafka 메시지 발행 실패: %v", err)
	}

	log.Printf("메시지가 Kafka 토픽 '%s'에 발행되었습니다: %s", topic, message.MessageId)
	return nil
}

// 채팅방 이벤트를 Kafka 토픽에 발행
func (p *KafkaProducer) PublishRoomEvent(ctx context.Context, topic string, eventType string, roomID string, userID string, data map[string]interface{}) error {
	event := map[string]interface{}{
		"event_type": eventType,
		"room_id":    roomID,
		"user_id":    userID,
		"timestamp":  time.Now().Unix(),
		"data":       data,
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("이벤트 직렬화 실패: %v", err)
	}

	err = p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(roomID),
		Value: eventBytes,
		Time:  time.Now(),
	})

	if err != nil {
		return fmt.Errorf("Kafka 이벤트 발행 실패: %v", err)
	}

	log.Printf("이벤트가 Kafka 토픽 '%s'에 발행되었습니다: %s", topic, eventType)
	return nil
}

// 프로듀서 종료
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
} 
