// 채팅 앱 클래스
class ChatApp {
    constructor() {
        this.userId = null;
        this.username = null;
        this.currentRoomId = null;
        this.currentRoomName = null;
        this.ws = null;
        this.rooms = [];
        this.users = [];
        
        this.init();
    }

    init() {
        this.bindEvents();
        this.loadStoredUser();
    }

    bindEvents() {
        // 로그인 이벤트
        document.getElementById('login-btn').addEventListener('click', () => this.handleLogin());
        document.getElementById('username-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.handleLogin();
        });

        // 채팅 이벤트
        document.getElementById('send-btn').addEventListener('click', () => this.sendMessage());
        document.getElementById('message-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.sendMessage();
        });

        // 방 관리 이벤트
        document.getElementById('create-room-btn').addEventListener('click', () => this.showCreateRoomModal());
        document.getElementById('create-room-confirm').addEventListener('click', () => this.createRoom());
        document.getElementById('create-room-cancel').addEventListener('click', () => this.hideCreateRoomModal());
        document.getElementById('leave-room-btn').addEventListener('click', () => this.leaveRoom());

        // 모달 이벤트
        document.getElementById('room-name-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.createRoom();
        });
    }

    // 저장된 사용자 정보 로드
    loadStoredUser() {
        const storedUser = localStorage.getItem('chatUser');
        if (storedUser) {
            const user = JSON.parse(storedUser);
            document.getElementById('username-input').value = user.username;
            // 저장된 사용자 ID가 있으면 복원
            if (user.userId) {
                this.userId = user.userId;
                this.username = user.username;
                console.log('저장된 사용자 정보 복원:', user);
            }
        }
    }

    // 로그인 처리
    async handleLogin() {
        const username = document.getElementById('username-input').value.trim();
        if (!username) {
            alert('사용자 이름을 입력해주세요.');
            return;
        }

        try {
            console.log('로그인 시도:', username);
            const response = await fetch('/api/register', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ username }),
            });

            const data = await response.json();
            console.log('로그인 응답:', data);
            
            if (data.success) {
                this.userId = data.user_id || data.userId;
                this.username = username;
                
                console.log('사용자 ID:', this.userId);
                console.log('사용자명:', this.username);
                
                // 사용자 정보 저장
                localStorage.setItem('chatUser', JSON.stringify({ username, userId: this.userId }));
                
                this.showChatScreen();
                
                // 채팅방 목록과 사용자 목록 로드
                await this.loadRooms();
                await this.loadUsers();
                
                // 기본 방에 자동 참여 (채팅방 목록 로드 후)
                console.log('기본 방 참여 시도...');
                await this.joinRoom('general', '일반');
            } else {
                alert(data.error || '로그인에 실패했습니다.');
            }
        } catch (error) {
            console.error('로그인 오류:', error);
            alert('로그인 중 오류가 발생했습니다.');
        }
    }

    // 채팅 화면 표시
    showChatScreen() {
        document.getElementById('login-screen').classList.add('hidden');
        document.getElementById('chat-screen').classList.remove('hidden');
        
        // 사용자 정보 표시
        document.getElementById('current-username').textContent = this.username;
        document.getElementById('user-avatar-text').textContent = this.username.charAt(0).toUpperCase();
    }

    // WebSocket 연결
    connectWebSocket() {
        if (!this.userId || !this.username) {
            console.error('사용자 정보가 없어 WebSocket 연결을 할 수 없습니다.');
            return;
        }

        // 기존 연결이 있으면 닫기
        if (this.ws && this.ws.readyState !== WebSocket.CLOSED) {
            console.log('기존 WebSocket 연결을 닫습니다.');
            this.ws.close();
        }

        const roomId = this.currentRoomId || 'general';
        const wsUrl = `ws://${window.location.host}/ws?userId=${this.userId}&username=${encodeURIComponent(this.username)}&roomId=${roomId}`;
        
        console.log('WebSocket 연결 시도:', wsUrl);
        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('WebSocket 연결됨 - 방:', roomId);
        };

        this.ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                console.log('WebSocket 메시지 수신:', data);
                this.handleWebSocketMessage(data);
            } catch (error) {
                console.error('WebSocket 메시지 파싱 오류:', error);
            }
        };

        this.ws.onclose = (event) => {
            console.log('WebSocket 연결 끊어짐:', event.code, event.reason);
            // 재연결 시도 (방에 참여 중인 경우에만)
            if (this.currentRoomId && event.code !== 1000) {
                console.log('WebSocket 재연결 시도...');
                setTimeout(() => {
                    if (this.currentRoomId) {
                        this.connectWebSocket();
                    }
                }, 3000);
            }
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket 오류:', error);
        };
    }

    // WebSocket 메시지 처리
    handleWebSocketMessage(data) {
        console.log('WebSocket 메시지 수신:', data);
        
        if (data.type === 'message') {
            this.addMessage(data);
        } else if (data.type === 'room_update') {
            this.handleRoomUpdate(data.room);
        } else {
            console.log('알 수 없는 메시지 타입:', data.type);
        }
    }

    // 방 업데이트 처리
    handleRoomUpdate(roomData) {
        console.log('방 업데이트 수신:', roomData);
        
        // 기존 방 목록에서 같은 ID의 방이 있는지 확인
        const existingRoomIndex = this.rooms.findIndex(room => 
            room.room_id === roomData.room_id || room.roomId === roomData.room_id
        );
        
        if (existingRoomIndex >= 0) {
            // 기존 방 업데이트
            this.rooms[existingRoomIndex] = roomData;
        } else {
            // 새 방 추가
            this.rooms.push(roomData);
        }
        
        this.renderRoomList();
    }

    // 채팅방 목록 로드
    async loadRooms() {
        try {
            const response = await fetch('/api/rooms');
            const data = await response.json();
            console.log('채팅방 목록 응답:', data);
            
            if (data.rooms && Array.isArray(data.rooms)) {
                this.rooms = data.rooms;
            } else if (Array.isArray(data)) {
                this.rooms = data;
            } else {
                this.rooms = [];
            }
            this.renderRoomList();
        } catch (error) {
            console.error('채팅방 목록 로드 오류:', error);
            this.rooms = [];
            this.renderRoomList();
        }
    }

    // 사용자 목록 로드
    async loadUsers() {
        try {
            const response = await fetch('/api/users');
            const data = await response.json();
            console.log('사용자 목록 응답:', data);
            
            if (data.users && Array.isArray(data.users)) {
                this.users = data.users;
            } else if (Array.isArray(data)) {
                this.users = data;
            } else {
                this.users = [];
            }
            this.renderUserList();
        } catch (error) {
            console.error('사용자 목록 로드 오류:', error);
            this.users = [];
            this.renderUserList();
        }
    }

    // 채팅방 목록 렌더링
    renderRoomList() {
        const roomList = document.getElementById('room-list');
        roomList.innerHTML = '';

        if (!this.rooms || this.rooms.length === 0) {
            roomList.innerHTML = '<div class="no-rooms">채팅방이 없습니다</div>';
            return;
        }

        this.rooms.forEach(room => {
            const roomElement = document.createElement('div');
            roomElement.className = 'room-item';
            
            // proto 파일의 필드명에 맞게 수정
            const roomId = room.room_id || room.roomId || room.id;
            const roomName = room.name || room.roomName;
            const userCount = room.user_count || room.userCount || (room.users ? room.users.length : 0);
            const creatorId = room.creator_id || room.creatorId;

            if (roomId === this.currentRoomId) {
                roomElement.classList.add('active');
            }

            let html = `
                <div class="room-name">${roomName}</div>
                <div class="room-count">${userCount}명</div>
                ${roomId === this.currentRoomId ? '<div class="current-indicator">참여 중</div>' : ''}
            `;

            // 내가 만든 방이면 삭제 버튼 추가
            if (creatorId === this.userId && roomId !== 'general') {
                html += `<button class="delete-room-btn" title="방 삭제">삭제</button>`;
            }

            roomElement.innerHTML = html;
            roomElement.addEventListener('click', (e) => {
                // 삭제 버튼 클릭 시 이벤트 버블링 방지
                if (e.target.classList.contains('delete-room-btn')) return;
                this.joinRoom(roomId, roomName);
            });

            // 삭제 버튼 이벤트
            const deleteBtn = roomElement.querySelector('.delete-room-btn');
            if (deleteBtn) {
                deleteBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    if (confirm('정말 이 방을 삭제하시겠습니까?')) {
                        await this.deleteRoom(roomId);
                    }
                });
            }

            roomList.appendChild(roomElement);
        });
    }

    // 사용자 목록 렌더링
    renderUserList() {
        const userList = document.getElementById('user-list');
        userList.innerHTML = '';

        if (!this.users || this.users.length === 0) {
            userList.innerHTML = '<div class="no-users">사용자가 없습니다</div>';
            return;
        }

        this.users.forEach(user => {
            const userElement = document.createElement('div');
            userElement.className = 'user-item';

            // proto 파일의 필드명에 맞게 수정
            const username = user.username || user.name;
            const isOnline = user.online !== undefined ? user.online : true;

            userElement.innerHTML = `
                <div class="user-name">${username}</div>
                <div class="user-status-indicator ${isOnline ? '' : 'offline'}"></div>
            `;

            userList.appendChild(userElement);
        });
    }

    // 방 참여 성공 처리
    handleRoomJoinSuccess(roomId, roomName) {
        this.currentRoomId = roomId;
        this.currentRoomName = roomName;
        
        // 새로운 WebSocket 연결
        this.connectWebSocket();
        
        this.updateChatHeader();
        this.clearMessages();
        this.enableChatInput();
        this.renderRoomList();
        
        // 방 목록을 주기적으로 업데이트하여 새로 생성된 방들이 반영되도록 함
        setTimeout(() => this.loadRooms(), 1000);
    }

    // 채팅방 참여
    async joinRoom(roomId, roomName) {
        if (!roomId) {
            console.error('roomId가 제공되지 않았습니다.');
            return;
        }
        
        if (this.currentRoomId === roomId) {
            console.log('이미 해당 방에 참여 중입니다:', roomId);
            return;
        }
        
        console.log('=== joinRoom 시작 ===');
        console.log('현재 방:', this.currentRoomId);
        console.log('참여하려는 방:', roomId);
        console.log('사용자 ID:', this.userId);

        try {
            // 기존 WebSocket 연결 정리
            if (this.ws && this.ws.readyState !== WebSocket.CLOSED) {
                console.log('기존 WebSocket 연결을 닫습니다.');
                this.ws.close();
            }

            // 기존 방에서 나가기 (다른 방에 참여 중인 경우)
            if (this.currentRoomId && this.currentRoomId !== roomId) {
                console.log('기존 방에서 나가기 시도:', this.currentRoomId);
                await this.leaveCurrentRoom();
            }

            console.log('join-room API 호출:', { userId: this.userId, roomId: roomId });
            const response = await fetch('/api/join-room', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    userId: this.userId,
                    roomId: roomId,
                }),
            });

            const data = await response.json();
            console.log('채팅방 참여 응답:', data);
            
            if (data.success) {
                console.log('채팅방 참여 성공!');
                this.handleRoomJoinSuccess(roomId, roomName);
            } else {
                console.error('채팅방 참여 실패:', data.error);
                
                // "이미 참여 중" 오류인 경우, 서버 상태를 강제로 동기화
                if (data.error && data.error.includes('이미 참여 중')) {
                    console.log('서버 상태 동기화 시도...');
                    this.handleRoomJoinSuccess(roomId, roomName);
                } else {
                    alert(data.error || '채팅방 참여에 실패했습니다.');
                }
            }
        } catch (error) {
            console.error('채팅방 참여 오류:', error);
            alert('채팅방 참여 중 오류가 발생했습니다.');
        }
        console.log('=== joinRoom 종료 ===');
    }

    // 현재 방에서 나가기 (API 호출 없이 내부 상태만 정리)
    async leaveCurrentRoom() {
        if (!this.currentRoomId) {
            console.log('나갈 방이 없습니다.');
            return;
        }

        console.log('=== leaveCurrentRoom 시작 ===');
        console.log('나가려는 방:', this.currentRoomId);

        try {
            const response = await fetch('/api/leave-room', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    userId: this.userId,
                    roomId: this.currentRoomId,
                }),
            });

            const data = await response.json();
            console.log('기존 방 나가기 응답:', data);
            
            if (data.success) {
                console.log('기존 방에서 성공적으로 나갔습니다:', this.currentRoomId);
            } else {
                console.warn('기존 방 나가기 실패:', data.error);
                // "참여하지 않은 채팅방" 오류는 무시 (이미 나간 상태)
                if (data.error && data.error.includes('참여하지 않은 채팅방')) {
                    console.log('이미 나간 방이므로 무시합니다.');
                }
            }
        } catch (error) {
            console.error('기존 방 나가기 오류:', error);
        }
        console.log('=== leaveCurrentRoom 종료 ===');
    }

    // 채팅방 나가기
    async leaveRoom() {
        if (!this.currentRoomId) return;

        try {
            const response = await fetch('/api/leave-room', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    userId: this.userId,
                    roomId: this.currentRoomId,
                }),
            });

            const data = await response.json();
            console.log('채팅방 나가기 응답:', data);
            
            if (data.success) {
                // WebSocket 연결 정리
                if (this.ws && this.ws.readyState !== WebSocket.CLOSED) {
                    console.log('WebSocket 연결을 닫습니다.');
                    this.ws.close();
                }
                
                this.currentRoomId = null;
                this.currentRoomName = null;
                
                this.updateChatHeader();
                this.clearMessages();
                this.disableChatInput();
                this.renderRoomList();
                this.loadRooms();
            } else {
                alert(data.error || '채팅방 나가기에 실패했습니다.');
            }
        } catch (error) {
            console.error('채팅방 나가기 오류:', error);
            alert('채팅방 나가기 중 오류가 발생했습니다.');
        }
    }

    // 메시지 전송
    async sendMessage() {
        const messageInput = document.getElementById('message-input');
        const message = messageInput.value.trim();
        
        if (!message || !this.currentRoomId) return;

        try {
            const response = await fetch('/api/send-message', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    userId: this.userId,
                    roomId: this.currentRoomId,
                    message: message,
                }),
            });

            const data = await response.json();
            console.log('메시지 전송 응답:', data);
            
            if (data.success) {
                messageInput.value = '';
                // WebSocket을 통해 메시지가 전송되므로 여기서는 입력만 클리어
            } else {
                alert(data.error || '메시지 전송에 실패했습니다.');
            }
        } catch (error) {
            console.error('메시지 전송 오류:', error);
            alert('메시지 전송 중 오류가 발생했습니다.');
        }
    }

    // 메시지 추가
    addMessage(data) {
        const messagesContainer = document.getElementById('chat-messages');
        const isOwnMessage = data.userId === this.userId;
        
        const messageElement = document.createElement('div');
        messageElement.className = `message ${isOwnMessage ? 'own' : ''}`;
        
        const timestamp = new Date(data.timestamp * 1000).toLocaleTimeString('ko-KR', {
            hour: '2-digit',
            minute: '2-digit'
        });

        messageElement.innerHTML = `
            <div class="message-avatar">${data.username.charAt(0).toUpperCase()}</div>
            <div class="message-content">
                <div class="message-text">${this.escapeHtml(data.message)}</div>
                <span class="message-time">${timestamp}</span>
            </div>
        `;

        messagesContainer.appendChild(messageElement);
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }

    // HTML 이스케이프
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // 채팅 헤더 업데이트
    updateChatHeader() {
        const roomNameElement = document.getElementById('current-room-name');
        const roomCountElement = document.getElementById('room-user-count');
        const leaveButton = document.getElementById('leave-room-btn');

        if (this.currentRoomName) {
            roomNameElement.textContent = this.currentRoomName;
            const room = this.rooms.find(r => r.roomId === this.currentRoomId);
            roomCountElement.textContent = room ? `${room.userCount}명 참여 중` : '';
            leaveButton.disabled = false;
        } else {
            roomNameElement.textContent = '채팅방을 선택하세요';
            roomCountElement.textContent = '';
            leaveButton.disabled = true;
        }
    }

    // 메시지 영역 클리어
    clearMessages() {
        const messagesContainer = document.getElementById('chat-messages');
        messagesContainer.innerHTML = `
            <div class="welcome-message">
                <h3>채팅방에 참여하세요!</h3>
                <p>왼쪽에서 채팅방을 선택하거나 새로운 방을 만들어보세요.</p>
            </div>
        `;
    }

    // 채팅 입력 활성화
    enableChatInput() {
        document.getElementById('message-input').disabled = false;
        document.getElementById('send-btn').disabled = false;
    }

    // 채팅 입력 비활성화
    disableChatInput() {
        document.getElementById('message-input').disabled = true;
        document.getElementById('send-btn').disabled = true;
    }

    // 방 만들기 모달 표시
    showCreateRoomModal() {
        document.getElementById('create-room-modal').classList.remove('hidden');
        document.getElementById('room-name-input').focus();
    }

    // 방 만들기 모달 숨기기
    hideCreateRoomModal() {
        document.getElementById('create-room-modal').classList.add('hidden');
        document.getElementById('room-name-input').value = '';
    }

    // 방 만들기
    async createRoom() {
        const roomName = document.getElementById('room-name-input').value.trim();
        if (!roomName) {
            alert('방 이름을 입력해주세요.');
            return;
        }

        console.log('방 만들기 시도:', { creatorId: this.userId, name: roomName });

        try {
            const response = await fetch('/api/create-room', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    creatorId: this.userId,
                    name: roomName,
                }),
            });

            const data = await response.json();
            console.log('방 만들기 응답:', data);
            
            if (data.success) {
                this.hideCreateRoomModal();
                const newRoomId = data.room_id || data.roomId;
                // 방 생성 후 바로 참여
                await this.joinRoom(newRoomId, roomName);
            } else {
                alert(data.error || '채팅방 생성에 실패했습니다.');
            }
        } catch (error) {
            console.error('채팅방 생성 오류:', error);
            alert('채팅방 생성 중 오류가 발생했습니다.');
        }
    }

    // 방 삭제
    async deleteRoom(roomId) {
        try {
            const response = await fetch('/api/delete-room', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ userId: this.userId, roomId })
            });
            const data = await response.json();
            if (data.success) {
                await this.loadRooms();
                // 삭제한 방에 참여 중이었다면 일반방으로 이동
                if (this.currentRoomId === roomId) {
                    await this.joinRoom('general', '일반');
                }
            } else {
                alert(data.error || '방 삭제에 실패했습니다.');
            }
        } catch (error) {
            alert('방 삭제 중 오류가 발생했습니다.');
        }
    }
}

// 앱 시작
document.addEventListener('DOMContentLoaded', () => {
    new ChatApp();
}); 
