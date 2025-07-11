package main

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/Kim-Yukyung/grpc-kafka-chat/internal/chat"
	"github.com/Kim-Yukyung/grpc-kafka-chat/internal/websocket"
	"github.com/Kim-Yukyung/grpc-kafka-chat/proto"
	"google.golang.org/grpc"
)

const (
	grpcPort = ":50051"
	webPort  = ":8081"
)

var (
	// Kafka 브로커 설정
	kafkaBrokers = []string{"localhost:9092"}
)

func main() {
	// WebSocket 허브 생성
	hub := websocket.NewHub()
	go hub.Run()

	// gRPC 서버 시작 (별도 고루틴에서)
	go startGRPCServer(hub)

	// API 서버 생성
	apiServer, err := websocket.NewAPIServer("localhost"+grpcPort, hub)
	if err != nil {
		log.Fatalf("API 서버 생성 실패: %v", err)
	}

	// 라우터 설정
	router := apiServer.SetupRoutes()

	log.Printf("웹 서버가 포트 %s에서 시작되었습니다", webPort)
	log.Printf("gRPC 서버: localhost%s", grpcPort)
	log.Printf("웹 브라우저에서 http://localhost%s 접속", webPort)
	log.Printf("서버를 중지하려면 Ctrl+C를 누르세요")

	// HTTP 서버 시작
	if err := http.ListenAndServe(webPort, router); err != nil {
		log.Fatalf("HTTP 서버 실행 실패: %v", err)
	}
}

// gRPC 서버 시작
func startGRPCServer(hub *websocket.Hub) {
	// 채팅 서비스 생성
	chatService := chat.NewChatService(kafkaBrokers)

	// WebSocket 허브를 채팅 서비스에 연결
	chatService.SetWebSocketHub(hub)

	// gRPC 서버 생성
	grpcServer := grpc.NewServer()
	proto.RegisterChatServiceServer(grpcServer, chatService)

	// Kafka 컨슈머 시작 (Kafka가 없어도 에러 없이 처리)
	if len(kafkaBrokers) > 0 {
		if err := chatService.StartKafkaConsumer(context.Background()); err != nil {
			log.Printf("Kafka 컨슈머 시작 실패: %v", err)
		}
	} else {
		log.Printf("Kafka 브로커가 설정되지 않아 Kafka 기능을 비활성화합니다.")
	}

	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("포트 %s에서 리스닝 실패: %v", grpcPort, err)
	}

	log.Printf("gRPC 서버가 포트 %s에서 시작되었습니다", grpcPort)
	if len(kafkaBrokers) > 0 {
		log.Printf("Kafka 브로커: %v", kafkaBrokers)
	} else {
		log.Printf("Kafka 없이 로컬 모드로 실행 중")
	}

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC 서버 실행 실패: %v", err)
	}
} 
