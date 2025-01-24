package network

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Unix系システムでネットワークインターフェースを操作する為の構造体
type ifreq struct {
	ifrName  [16]byte
	ifrFlags uint16
}

const (
	TUNSETIFF   = 0x400454ca // TUN/TAPデバイスの設定を行うためのioctlコマンド
	IFF_TUN     = 0x0001     // TUNデバイスを指定するフラグ
	IFF_NO_PI   = 0x1000     // パケットの先頭に追加されるパケット情報を省略するフラグ
	PACKET_SIZE = 2048       // パケットのデフォルトサイズ
	QUEUE_SIZE  = 100        // パケットキューのサイズ
)

type Packet struct {
	Buf []byte  // パケットのデータを格納するバイトスライス
	N   uintptr // パケットのサイズ
}

type NetDevice struct {
	file          *os.File    // ネットワークデバイスのファイルディスクリプタ
	incomingQueue chan Packet // パケットを受信するキュー
	outgoingQueue chan Packet // パケットを送信するキュー
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewTun() (*NetDevice, error) {
	file, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open /dev/net/tun: %w", err)
	}

	ifr := ifreq{}
	copy(ifr.ifrName[:], []byte("tun0"))
	ifr.ifrFlags = IFF_TUN | IFF_NO_PI
	_, _, sysErr := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(TUNSETIFF), uintptr(unsafe.Pointer(&ifr)))
	if sysErr != 0 {
		return nil, fmt.Errorf("failed to ioctl: %w", sysErr)
	}

	return &NetDevice{
		file:          file,
		incomingQueue: make(chan Packet, QUEUE_SIZE),
		outgoingQueue: make(chan Packet, QUEUE_SIZE),
	}, nil
}

func (d *NetDevice) read(buf []byte) (uintptr, error) {
	n, _, sysErr := syscall.Syscall(syscall.SYS_READ, d.file.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if sysErr != 0 {
		return 0, fmt.Errorf("failed to read: %w", sysErr)
	}
	return n, nil
}

func (d *NetDevice) write(buf []byte) (uintptr, error) {
	n, _, sysErr := syscall.Syscall(syscall.SYS_WRITE, d.file.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if sysErr != 0 {
		return 0, fmt.Errorf("failed to write: %w", sysErr)
	}
	return n, nil
}

func (d *NetDevice) Bind() {
	d.ctx, d.cancel = context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			default:
				buf := make([]byte, PACKET_SIZE)
				n, err := d.read(buf)
				if err != nil {
					fmt.Println(err)
					continue
				}
				d.incomingQueue <- Packet{Buf: buf, N: n}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case packet := <-d.outgoingQueue:
				_, err := d.write(packet.Buf[:packet.N])
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	}()
}

func (d *NetDevice) Read() (Packet, error) {
	packet, ok := <-d.incomingQueue
	if !ok {
		return Packet{}, fmt.Errorf("failed to read packet")
	}
	return packet, nil
}

func (d *NetDevice) Write(packet Packet) error {
	select {
	case d.outgoingQueue <- packet:
		return nil
	case <-d.ctx.Done():
		return fmt.Errorf("failed to write packet")
	}
}
