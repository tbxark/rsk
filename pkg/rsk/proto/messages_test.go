package proto

import (
	"bytes"
	"testing"
)

func TestHelloRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		hello   Hello
		wantErr bool
	}{
		{
			name: "valid hello with single port",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte("test-token-123"),
				Ports:   []uint16{20000},
				Name:    "test-client",
			},
			wantErr: false,
		},
		{
			name: "valid hello with multiple ports",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte("another-token"),
				Ports:   []uint16{20000, 20001, 20002},
				Name:    "multi-port-client",
			},
			wantErr: false,
		},
		{
			name: "valid hello with empty name",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte("token"),
				Ports:   []uint16{30000},
				Name:    "",
			},
			wantErr: false,
		},
		{
			name: "valid hello with max ports",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte("token"),
				Ports:   []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				Name:    "max-ports",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Write
			err := WriteHello(&buf, tt.hello)
			if (err != nil) != tt.wantErr {
				t.Fatalf("WriteHello() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Read
			got, err := ReadHello(&buf)
			if err != nil {
				t.Fatalf("ReadHello() error = %v", err)
			}

			// Compare
			if string(got.Magic[:]) != string(tt.hello.Magic[:]) {
				t.Errorf("Magic mismatch: got %v, want %v", got.Magic, tt.hello.Magic)
			}
			if got.Version != tt.hello.Version {
				t.Errorf("Version mismatch: got %v, want %v", got.Version, tt.hello.Version)
			}
			if string(got.Token) != string(tt.hello.Token) {
				t.Errorf("Token mismatch: got %v, want %v", got.Token, tt.hello.Token)
			}
			if len(got.Ports) != len(tt.hello.Ports) {
				t.Errorf("Ports length mismatch: got %v, want %v", len(got.Ports), len(tt.hello.Ports))
			}
			for i := range got.Ports {
				if got.Ports[i] != tt.hello.Ports[i] {
					t.Errorf("Port[%d] mismatch: got %v, want %v", i, got.Ports[i], tt.hello.Ports[i])
				}
			}
			if got.Name != tt.hello.Name {
				t.Errorf("Name mismatch: got %v, want %v", got.Name, tt.hello.Name)
			}
		})
	}
}

func TestHelloRespRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		helloResp HelloResp
		wantErr   bool
	}{
		{
			name: "valid OK response",
			helloResp: HelloResp{
				Version:       0x01,
				Status:        StatusOK,
				AcceptedPorts: []uint16{20000, 20001},
				Message:       "success",
			},
			wantErr: false,
		},
		{
			name: "valid error response with no ports",
			helloResp: HelloResp{
				Version:       0x01,
				Status:        StatusAuthFail,
				AcceptedPorts: []uint16{},
				Message:       "authentication failed",
			},
			wantErr: false,
		},
		{
			name: "valid response with empty message",
			helloResp: HelloResp{
				Version:       0x01,
				Status:        StatusOK,
				AcceptedPorts: []uint16{30000},
				Message:       "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Write
			err := WriteHelloResp(&buf, tt.helloResp)
			if (err != nil) != tt.wantErr {
				t.Fatalf("WriteHelloResp() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Read
			got, err := ReadHelloResp(&buf)
			if err != nil {
				t.Fatalf("ReadHelloResp() error = %v", err)
			}

			// Compare
			if got.Version != tt.helloResp.Version {
				t.Errorf("Version mismatch: got %v, want %v", got.Version, tt.helloResp.Version)
			}
			if got.Status != tt.helloResp.Status {
				t.Errorf("Status mismatch: got %v, want %v", got.Status, tt.helloResp.Status)
			}
			if len(got.AcceptedPorts) != len(tt.helloResp.AcceptedPorts) {
				t.Errorf("AcceptedPorts length mismatch: got %v, want %v", len(got.AcceptedPorts), len(tt.helloResp.AcceptedPorts))
			}
			for i := range got.AcceptedPorts {
				if got.AcceptedPorts[i] != tt.helloResp.AcceptedPorts[i] {
					t.Errorf("AcceptedPorts[%d] mismatch: got %v, want %v", i, got.AcceptedPorts[i], tt.helloResp.AcceptedPorts[i])
				}
			}
			if got.Message != tt.helloResp.Message {
				t.Errorf("Message mismatch: got %v, want %v", got.Message, tt.helloResp.Message)
			}
		})
	}
}

func TestConnectReqRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "valid IPv4 address",
			addr:    "192.168.1.1:8080",
			wantErr: false,
		},
		{
			name:    "valid IPv6 address RFC3986 format",
			addr:    "[2001:db8::1]:443",
			wantErr: false,
		},
		{
			name:    "valid domain name",
			addr:    "example.com:80",
			wantErr: false,
		},
		{
			name:    "valid localhost",
			addr:    "localhost:3000",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Write
			err := WriteConnectReq(&buf, tt.addr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("WriteConnectReq() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Read
			got, err := ReadConnectReq(&buf)
			if err != nil {
				t.Fatalf("ReadConnectReq() error = %v", err)
			}

			// Compare
			if got != tt.addr {
				t.Errorf("Address mismatch: got %v, want %v", got, tt.addr)
			}
		})
	}
}

func TestHelloValidation(t *testing.T) {
	tests := []struct {
		name    string
		hello   Hello
		wantErr error
	}{
		{
			name: "invalid magic",
			hello: Hello{
				Magic:   [4]byte{'B', 'A', 'D', '!'},
				Version: 0x01,
				Token:   []byte("token"),
				Ports:   []uint16{20000},
				Name:    "test",
			},
			wantErr: ErrInvalidMagic,
		},
		{
			name: "invalid version",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x02,
				Token:   []byte("token"),
				Ports:   []uint16{20000},
				Name:    "test",
			},
			wantErr: ErrInvalidVersion,
		},
		{
			name: "token too short",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte{},
				Ports:   []uint16{20000},
				Name:    "test",
			},
			wantErr: ErrInvalidTokenLen,
		},
		{
			name: "no ports",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte("token"),
				Ports:   []uint16{},
				Name:    "test",
			},
			wantErr: ErrInvalidPortCount,
		},
		{
			name: "too many ports",
			hello: Hello{
				Magic:   [4]byte{'R', 'S', 'K', '1'},
				Version: 0x01,
				Token:   []byte("token"),
				Ports:   []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17},
				Name:    "test",
			},
			wantErr: ErrInvalidPortCount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteHello(&buf, tt.hello)
			if err != tt.wantErr {
				t.Errorf("WriteHello() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
