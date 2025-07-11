package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
	pb "github.com/Kim-Yukyung/grpc-kafka-chat/proto"
)

// 사용자 관리 생성
func NewUsers() *Users {
	return &Users{
		users: make(map[string]*User),
	}
}

// 사용자 등록
func (u *Users) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	u.usersMux.Lock()
	defer u.usersMux.Unlock()

	// 사용자 이름 중복 체크
	for _, user := range u.users {
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
	
	u.users[userID] = user
	
	return &pb.RegisterUserResponse{
		Success: true,
		UserId:  userID,
	}, nil
}

// 사용자 존재 확인
func (u *Users) GetUser(userID string) (*User, bool) {
	u.usersMux.RLock()
	defer u.usersMux.RUnlock()
	
	user, exists := u.users[userID]
	return user, exists
}

// 사용자 목록 조회
func (u *Users) GetUsers(ctx context.Context, req *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	u.usersMux.RLock()
	defer u.usersMux.RUnlock()
	
	var users []*pb.User
	for _, user := range u.users {
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

// 사용자 상태 업데이트
func (u *Users) UpdateUserStatus(ctx context.Context, req *pb.UpdateUserStatusRequest) (*pb.UpdateUserStatusResponse, error) {
	u.usersMux.Lock()
	defer u.usersMux.Unlock()
	
	user, exists := u.users[req.UserId]
	if !exists {
		return &pb.UpdateUserStatusResponse{
			Success: false,
			Error:   "존재하지 않는 사용자입니다",
		}, nil
	}
	
	user.Online = req.Online
	
	return &pb.UpdateUserStatusResponse{
		Success: true,
	}, nil
}

// 사용자 이름 목록 반환
func (u *Users) GetUsernames(users map[string]*User) []string {
	var usernames []string
	for _, user := range users {
		usernames = append(usernames, user.Username)
	}
	return usernames
} 
