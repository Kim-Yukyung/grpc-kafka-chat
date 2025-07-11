package chat

import (
	"context"
	"log"

	"github.com/Kim-Yukyung/grpc-kafka-chat/internal/kafka"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// Kafka 관리 생성
func NewKafka(kafkaBrokers []string) *Kafka {
	k := &Kafka{
		kafkaBrokers: kafkaBrokers,
		messageTopic: "chat-messages",
		eventTopic:   "chat-events",
	}
	
	// Kafka가 설정된 경우 Kafka 프로듀서 초기화
	if len(kafkaBrokers) > 0 {
		k.producer = kafka.NewKafkaProducer(kafkaBrokers)
		log.Printf("Kafka 프로듀서가 초기화되었습니다. 브로커: %v", kafkaBrokers)
	} else {
		log.Printf("Kafka 브로커가 설정되지 않아 Kafka 기능을 비활성화합니다.")
	}
	
	return k
}

// Kafka 컨슈머 시작
func (k *Kafka) StartKafkaConsumer(ctx context.Context) error {
	if len(k.kafkaBrokers) == 0 {
		log.Println("Kafka 브로커가 설정되지 않아 컨슈머를 시작하지 않습니다.")
		return nil
	}
	
	// 메시지 컨슈머 시작
	messageConsumer := kafka.NewKafkaConsumer(k.kafkaBrokers, k.messageTopic, "chat-service-group")
	
	// 고루틴으로 메시지 컨슈머 시작
	go func() {
		if err := messageConsumer.ConsumeMessages(ctx, k.handleKafkaMessage); err != nil {
			log.Printf("메시지 컨슈머 오류: %v", err)
		}
	}()
	
	// 이벤트 컨슈머 시작
	eventConsumer := kafka.NewKafkaConsumer(k.kafkaBrokers, k.eventTopic, "chat-service-events-group")
	
	// 고루틴으로 이벤트 컨슈머 시작
	go func() {
		if err := eventConsumer.ConsumeEvents(ctx, k.handleKafkaEvent); err != nil {
			log.Printf("이벤트 컨슈머 오류: %v", err)
		}
	}()
	
	log.Printf("Kafka 컨슈머가 시작되었습니다. 메시지 토픽: %s, 이벤트 토픽: %s", k.messageTopic, k.eventTopic)
	return nil
}

// Kafka에서 받은 메시지를 처리
func (k *Kafka) handleKafkaMessage(message *pb.ChatMessage) error {
	log.Printf("Kafka에서 메시지 수신: %s", message.MessageId)
	
	// Kafka에서 받은 메시지는 다시 브로드캐스트하지 않음
	// (이미 gRPC로 전송되었으므로 중복 방지)
	return nil
}

// Kafka에서 받은 이벤트를 처리
func (k *Kafka) handleKafkaEvent(eventType string, roomID string, userID string, data map[string]interface{}) error {
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

// 방 이벤트 발행
func (k *Kafka) PublishRoomEvent(ctx context.Context, eventType string, roomID string, userID string, data map[string]interface{}) error {
	if k.producer == nil {
		return nil
	}
	
	return k.producer.PublishRoomEvent(ctx, k.eventTopic, eventType, roomID, userID, data)
}

// 메시지 발행
func (k *Kafka) PublishMessage(ctx context.Context, message *pb.ChatMessage) error {
	if k.producer == nil {
		return nil
	}
	
	return k.producer.PublishMessage(ctx, k.messageTopic, message)
} 
