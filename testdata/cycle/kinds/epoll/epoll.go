package epoll

import "fx/socket"

func Wait() int { return socket.Limit }
