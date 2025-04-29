package sockets

func WithPingIntervalSec(p int) func(*Conn) {
	return func(s *Conn) {
		s.pingIntervalSecs = p
	}
}

func WithPingMsg(msg []byte) func(*Conn) {
	return func(s *Conn) {
		s.pingMsg = msg
	}
}

func InsecureSkipVerify() func(*Conn) {
	return func(s *Conn) {
		s.sslSkipVerify = true
	}
}

// func WithMatchMsg(f func([]byte, []byte) bool) func(*Conn) {
// 	return func(s *Conn) {
// 		s.matchMsg = f
// 	}
// }

// func WithMaxMessageSize(size int) func(*Conn) {
// 	return func(s *Conn) {
// 		s.maxMessageSize = size
// 	}
// }

func OnMessage(f func([]byte, Connection)) func(*Conn) {
	return func(s *Conn) {
		s.onMessage = f
	}
}

func OnError(f func(error)) func(*Conn) {
	return func(s *Conn) {
		s.onError = f
	}
}

func OnConnected(f func(Connection)) func(*Conn) {
	return func(s *Conn) {
		s.onConnected = f
	}
}
