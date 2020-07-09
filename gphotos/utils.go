package gphotos

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net"
)

func codeVerifier(length int) ([]byte, error) {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-._~"
	codeVerifier := make([]byte, length)
	buf := make([]byte, 16)
	i := 0
outer:
	for {
		_, err := rand.Read(buf)
		if err != nil {
			return nil, err
		}

		for _, b := range buf {
			b %= byte(len(chars))
			codeVerifier[i] = chars[b]
			i++
			if i == length {
				break outer
			}
		}
	}

	return codeVerifier, nil
}

func loopbackAddr() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	var loopback string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.IsLoopback() {
			if t4 := ipnet.IP.To4(); t4 != nil {
				loopback = t4.String()
				break
			}
		}
	}
	if loopback == "" {
		return loopback, errors.New("no loopback device found")
	}

	addr, err := net.ResolveTCPAddr("tcp", loopback+":0")
	if err != nil {
		return loopback, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return loopback, err
	}
	defer l.Close()

	return fmt.Sprintf("%s:%d", loopback, l.Addr().(*net.TCPAddr).Port), nil
}
