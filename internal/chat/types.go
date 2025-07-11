package chat

import (
	"sync"
	"time"

	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
	"github.com/Kim-Yukyung/grpc-kafka-chat/internal/kafka"
)

// ChatService 메인 서비스 구조체
type ChatService struct {
	pb.UnimplementedChatServiceServer
	
	// 사용자 관리
	users *Users
	
	// 채팅방 관리
	rooms *Rooms
	
	// 메시지 관리
	messages *Messages
	
	// 스트리밍 관리
	streams *Streams
	
	// Kafka 통합
	kafka *Kafka
	
	// WebSocket 허브
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

// 사용자 관리
type Users struct {
	users    map[string]*User
	usersMux sync.RWMutex
}

// 채팅방 관리
type Rooms struct {
	rooms    map[string]*Room
	roomsMux sync.RWMutex
}

// 메시지 관리
type Messages struct {
	producer *kafka.KafkaProducer
	wsHub    interface {
		BroadcastMessage(message *pb.ChatMessage)
	}
}

// 스트리밍 관리
type Streams struct {
	streams    map[string]map[string]chan *pb.ChatMessage
	streamsMux sync.RWMutex
}

// Kafka 관리
type Kafka struct {
	producer      *kafka.KafkaProducer
	consumer      *kafka.KafkaConsumer
	kafkaBrokers  []string
	messageTopic  string
	eventTopic    string
} 
