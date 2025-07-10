package kafka

import (
	"context"
	"encoding/json"
	"log"

	"github.com/segmentio/kafka-go"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

/*
	콜백 인터페이스 정의
*/

// 채팅 메시지를 처리하기 위한 콜백 함수
type MessageHandler func(message *pb.ChatMessage) error

// 일반 이벤트를 처리하기 위한 콜백 함수
type EventHandler func(eventType string, roomID string, userID string, data map[string]interface{}) error

// Kafka 토픽으로부터 메시지를 읽어오는 컨슈머 구조체
// 지속적으로 kafka 토픽으로부터 메시지를 읽어오고 처리
type KafkaConsumer struct {
	reader *kafka.Reader
}

// Kafka 메시지를 읽기 위한 새로운 컨슈머 인스턴스 생성
func NewKafkaConsumer(brokers []string, topic string, groupID string) *KafkaConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MaxBytes: 10e6, // 10MB
		Logger:   kafka.LoggerFunc(log.Printf),
	})

	return &KafkaConsumer{
		reader: reader,
	}
}

// 메시지를 읽고 핸들러에 전달
// go runtine처럼 무한 루프로 메시지를 읽고 처리
func (c *KafkaConsumer) ConsumeMessages(ctx context.Context, handler MessageHandler) error {
	log.Printf("메시지 소비를 시작합니다. 토픽: %s", c.reader.Config().Topic)

	for {
		select {
		case <-ctx.Done():
			log.Println("메시지 소비가 중단되었습니다.")
			return ctx.Err()
		default:
			// 메시지 읽기
			m, err := c.reader.ReadMessage(ctx)
			if err != nil {
				log.Printf("메시지 읽기 실패: %v", err)
				continue
			}

			// JSON 역직렬화
			var message pb.ChatMessage
			if err := json.Unmarshal(m.Value, &message); err != nil {
				log.Printf("메시지 역직렬화 실패: %v", err)
				continue
			}

			// 메시지 처리
			if err := handler(&message); err != nil {
				log.Printf("메시지 처리 실패: %v", err)
				continue
			}

			log.Printf("메시지 처리 완료: %s", message.MessageId)
		}
	}
}

// 이벤트를 읽고 핸들러에 전달
func (c *KafkaConsumer) ConsumeEvents(ctx context.Context, handler EventHandler) error {
	log.Printf("이벤트 소비를 시작합니다. 토픽: %s", c.reader.Config().Topic)

	for {
		select {
		case <-ctx.Done():
			log.Println("이벤트 소비가 중단되었습니다.")
			return ctx.Err()
		default:
			// 메시지 읽기
			m, err := c.reader.ReadMessage(ctx)
			if err != nil {
				log.Printf("이벤트 읽기 실패: %v", err)
				continue
			}

			// JSON 역직렬화
			var event map[string]interface{}
			if err := json.Unmarshal(m.Value, &event); err != nil {
				log.Printf("이벤트 역직렬화 실패: %v", err)
				continue
			}

			// 이벤트 데이터 추출
			eventType, ok := event["event_type"].(string)
			if !ok {
				log.Printf("이벤트 타입 추출 실패")
				continue
			}

			roomID, ok := event["room_id"].(string)
			if !ok {
				log.Printf("방 ID 추출 실패")
				continue
			}

			userID, ok := event["user_id"].(string)
			if !ok {
				log.Printf("사용자 ID 추출 실패")
				continue
			}

			data, ok := event["data"].(map[string]interface{})
			if !ok {
				data = make(map[string]interface{})
			}

			// 이벤트 처리
			if err := handler(eventType, roomID, userID, data); err != nil {
				log.Printf("이벤트 처리 실패: %v", err)
				continue
			}

			log.Printf("이벤트 처리 완료: %s", eventType)
		}
	}
}

// 컨슈머 종료
func (c *KafkaConsumer) Close() error {
	return c.reader.Close()
} 
