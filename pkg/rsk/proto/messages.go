package proto

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	MagicValue = "RSK1"
	Version    = 0x01
)

const (
	MaxTokenLen  = 255
	MinTokenLen  = 1
	MaxPortCount = 16
	MinPortCount = 1
	MaxNameLen   = 64
	MaxHelloSize = 2048
)

var (
	ErrInvalidMagic     = errors.New("invalid MAGIC field")
	ErrInvalidVersion   = errors.New("invalid VERSION field")
	ErrInvalidTokenLen  = errors.New("token length must be 1-255 bytes")
	ErrInvalidPortCount = errors.New("port count must be 1-16")
	ErrInvalidNameLen   = errors.New("name length must be 0-64 bytes")
	ErrMessageTooLarge  = errors.New("message exceeds maximum size")
)

// Hello represents the HELLO message.
type Hello struct {
	Magic   [4]byte  // "RSK1"
	Version uint8    // Protocol version
	Token   []byte   // Authentication token
	Ports   []uint16 // Ports to claim
	Name    string   // Client name
}

// WriteHello encodes and writes a HELLO message.
func WriteHello(w io.Writer, h Hello) error {
	if string(h.Magic[:]) != MagicValue {
		return ErrInvalidMagic
	}
	if h.Version != Version {
		return ErrInvalidVersion
	}
	if len(h.Token) < MinTokenLen || len(h.Token) > MaxTokenLen {
		return ErrInvalidTokenLen
	}
	if len(h.Ports) < MinPortCount || len(h.Ports) > MaxPortCount {
		return ErrInvalidPortCount
	}
	if len(h.Name) > MaxNameLen {
		return ErrInvalidNameLen
	}

	// Calculate total size
	totalSize := 4 + 1 + 1 + len(h.Token) + 1 + len(h.Ports)*2 + 1 + len(h.Name)
	if totalSize > MaxHelloSize {
		return ErrMessageTooLarge
	}

	if _, err := w.Write(h.Magic[:]); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, h.Version); err != nil {
		return err
	}

	tokenLen := uint8(len(h.Token))
	if err := binary.Write(w, binary.BigEndian, tokenLen); err != nil {
		return err
	}

	if _, err := w.Write(h.Token); err != nil {
		return err
	}

	portCnt := uint8(len(h.Ports))
	if err := binary.Write(w, binary.BigEndian, portCnt); err != nil {
		return err
	}

	for _, port := range h.Ports {
		if err := binary.Write(w, binary.BigEndian, port); err != nil {
			return err
		}
	}

	nameLen := uint8(len(h.Name))
	if err := binary.Write(w, binary.BigEndian, nameLen); err != nil {
		return err
	}

	if len(h.Name) > 0 {
		if _, err := w.Write([]byte(h.Name)); err != nil {
			return err
		}
	}

	return nil
}

// ReadHello reads and decodes a HELLO message.
func ReadHello(r io.Reader) (Hello, error) {
	var h Hello

	if _, err := io.ReadFull(r, h.Magic[:]); err != nil {
		return h, err
	}
	if string(h.Magic[:]) != MagicValue {
		return h, ErrInvalidMagic
	}

	// Read VERSION (1 byte)
	if err := binary.Read(r, binary.BigEndian, &h.Version); err != nil {
		return h, err
	}
	if h.Version != Version {
		return h, ErrInvalidVersion
	}

	// Read TOKEN_LEN (1 byte)
	var tokenLen uint8
	if err := binary.Read(r, binary.BigEndian, &tokenLen); err != nil {
		return h, err
	}
	if tokenLen < MinTokenLen || tokenLen > MaxTokenLen {
		return h, ErrInvalidTokenLen
	}

	h.Token = make([]byte, tokenLen)
	if _, err := io.ReadFull(r, h.Token); err != nil {
		return h, err
	}

	var portCnt uint8
	if err := binary.Read(r, binary.BigEndian, &portCnt); err != nil {
		return h, err
	}
	if portCnt < MinPortCount || portCnt > MaxPortCount {
		return h, ErrInvalidPortCount
	}

	h.Ports = make([]uint16, portCnt)
	for i := 0; i < int(portCnt); i++ {
		if err := binary.Read(r, binary.BigEndian, &h.Ports[i]); err != nil {
			return h, err
		}
	}

	var nameLen uint8
	if err := binary.Read(r, binary.BigEndian, &nameLen); err != nil {
		return h, err
	}
	if nameLen > MaxNameLen {
		return h, ErrInvalidNameLen
	}

	if nameLen > 0 {
		nameBytes := make([]byte, nameLen)
		if _, err := io.ReadFull(r, nameBytes); err != nil {
			return h, err
		}
		h.Name = string(nameBytes)
	}

	return h, nil
}

// Status codes for HELLO_RESP
const (
	StatusOK             = 0x00
	StatusAuthFail       = 0x01
	StatusBadRequest     = 0x02
	StatusPortForbidden  = 0x03
	StatusPortInUse      = 0x04
	StatusServerInternal = 0x05
)

const (
	MaxAcceptedPortCount = 16
	MaxMessageLen        = 255
)

var (
	ErrInvalidAcceptedPortCount = errors.New("accepted port count must be 0-16")
	ErrInvalidMessageLen        = errors.New("message length must be 0-255 bytes")
)

// HelloResp represents the HELLO_RESP message.
type HelloResp struct {
	Version       uint8    // Protocol version
	Status        uint8    // Status code
	AcceptedPorts []uint16 // Accepted ports
	Message       string   // Status message
}

// WriteHelloResp encodes and writes a HELLO_RESP message.
func WriteHelloResp(w io.Writer, h HelloResp) error {
	if h.Version != Version {
		return ErrInvalidVersion
	}
	if len(h.AcceptedPorts) > MaxAcceptedPortCount {
		return ErrInvalidAcceptedPortCount
	}
	if len(h.Message) > MaxMessageLen {
		return ErrInvalidMessageLen
	}

	if err := binary.Write(w, binary.BigEndian, h.Version); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, h.Status); err != nil {
		return err
	}

	acptCnt := uint8(len(h.AcceptedPorts))
	if err := binary.Write(w, binary.BigEndian, acptCnt); err != nil {
		return err
	}

	for _, port := range h.AcceptedPorts {
		if err := binary.Write(w, binary.BigEndian, port); err != nil {
			return err
		}
	}

	msgLen := uint8(len(h.Message))
	if err := binary.Write(w, binary.BigEndian, msgLen); err != nil {
		return err
	}

	if len(h.Message) > 0 {
		if _, err := w.Write([]byte(h.Message)); err != nil {
			return err
		}
	}

	return nil
}

// ReadHelloResp reads and decodes a HELLO_RESP message.
func ReadHelloResp(r io.Reader) (HelloResp, error) {
	var h HelloResp

	if err := binary.Read(r, binary.BigEndian, &h.Version); err != nil {
		return h, err
	}
	if h.Version != Version {
		return h, ErrInvalidVersion
	}

	if err := binary.Read(r, binary.BigEndian, &h.Status); err != nil {
		return h, err
	}

	var acptCnt uint8
	if err := binary.Read(r, binary.BigEndian, &acptCnt); err != nil {
		return h, err
	}
	if acptCnt > MaxAcceptedPortCount {
		return h, ErrInvalidAcceptedPortCount
	}

	if acptCnt > 0 {
		h.AcceptedPorts = make([]uint16, acptCnt)
		for i := 0; i < int(acptCnt); i++ {
			if err := binary.Read(r, binary.BigEndian, &h.AcceptedPorts[i]); err != nil {
				return h, err
			}
		}
	}

	var msgLen uint8
	if err := binary.Read(r, binary.BigEndian, &msgLen); err != nil {
		return h, err
	}
	if msgLen > MaxMessageLen {
		return h, ErrInvalidMessageLen
	}

	if msgLen > 0 {
		msgBytes := make([]byte, msgLen)
		if _, err := io.ReadFull(r, msgBytes); err != nil {
			return h, err
		}
		h.Message = string(msgBytes)
	}

	return h, nil
}

const (
	MaxAddrLen = 1024
	MinAddrLen = 1
)

var (
	ErrInvalidAddrLen = errors.New("address length must be 1-1024 bytes")
)

// ConnectReq represents the CONNECT_REQ message.
type ConnectReq struct {
	Addr string // Target address in "host:port" format
}

// WriteConnectReq encodes and writes a CONNECT_REQ message.
func WriteConnectReq(w io.Writer, addr string) error {
	if len(addr) < MinAddrLen || len(addr) > MaxAddrLen {
		return ErrInvalidAddrLen
	}

	addrLen := uint16(len(addr))
	if err := binary.Write(w, binary.BigEndian, addrLen); err != nil {
		return err
	}

	if _, err := w.Write([]byte(addr)); err != nil {
		return err
	}

	return nil
}

// ReadConnectReq reads and decodes a CONNECT_REQ message.
func ReadConnectReq(r io.Reader) (string, error) {
	var addrLen uint16
	if err := binary.Read(r, binary.BigEndian, &addrLen); err != nil {
		return "", err
	}
	if addrLen < MinAddrLen || addrLen > MaxAddrLen {
		return "", ErrInvalidAddrLen
	}

	addrBytes := make([]byte, addrLen)
	if _, err := io.ReadFull(r, addrBytes); err != nil {
		return "", err
	}

	return string(addrBytes), nil
}
