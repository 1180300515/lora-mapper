package driver

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (

	// Default TCP timeout is not set
	tcpTimeout     = 10 * time.Second
	tcpIdleTimeout = 60 * time.Second
)

// TCPClientHandler implements Packager and Transporter interface.
type TCPClientHandler struct {
	tcpPackager
	tcpTransporter
}

type ReceivePackage struct {
	data   []byte
	online byte
}

// NewTCPClientHandler allocates a new TCPClientHandler.
func NewTCPClientHandler(address string) *TCPClientHandler {
	h := &TCPClientHandler{}
	h.Address = address
	h.Timeout = tcpTimeout
	h.IdleTimeout = tcpIdleTimeout
	return h
}

// tcpPackager implements Packager interface.
type tcpPackager struct {

	//concentratorID
	ConcentratorID uint32
	//convertorID
	ConverterID uint32
	//datalength
	Datalength byte

	//convertor command

	//modbus part
	// Broadcast address is 0
	SlaveId byte
	//functioncode
	Functioncode byte
}

// Encode adds modbus application protocol header:
//  Transaction identifier: 2 bytes
//  Protocol identifier: 2 bytes
//  Length: 2 bytes
//  ConcentratorID : 4 bytes
//  ConvertorID:     4 bytes
//  Datalength:      1byte
//  slaveid: 1 byte
//  Function code: 1 byte
//  Data: n bytes + CRC + \r\n
func (mb *tcpPackager) Encode(data [5]byte) (adu []byte) {
	fmt.Println("encode ing")
	adu = make([]byte, 9+1+5+2)

	binary.BigEndian.PutUint32(adu[0:], uint32(mb.ConcentratorID))
	binary.BigEndian.PutUint32(adu[4:], uint32(mb.ConverterID))
	//datalength is the modbus command length
	datalength := byte(1 + 5 + 2)
	adu[8] = datalength
	//modbus commmand
	adu[9] = byte(mb.SlaveId)
	adu[10] = data[0]
	adu[11] = data[1]
	adu[12] = data[2]
	adu[13] = data[3]
	adu[14] = data[4]
	// Append crc
	var crc crc
	crc.reset().pushBytes(adu[9:15])
	checksum := crc.value()

	adu[15] = byte(checksum >> 8)
	adu[16] = byte(checksum)
	fmt.Printf("encode finish the package is %s \n", adu)
	return
}

// Decode extracts PDU from TCP frame:
//  version(1) + command(1) + concentratorid(4) + convertorid(4) + shortid(2)
//  channel(2) + SNR(1) + RSSI[0] (1) +RSSI[1](1) + NC(2) +timestamp(4) + online(1) + sum(2) + length(2)
//  data max240
func (mb *tcpPackager) Decode(adu []byte) (recv ReceivePackage) {
	fmt.Println("decode ing")
	recv.online = adu[23]
	length := binary.BigEndian.Uint16(adu[27:29])
	recv.data = adu[29 : 29+length]
	fmt.Println("decode finish the receice package is ", recv.data)
	return
}

// tcpTransporter implements Transporter interface.
type tcpTransporter struct {
	// Connect string
	Address string
	// Connect & Read timeout
	Timeout time.Duration
	// Idle timeout to close the connection
	IdleTimeout time.Duration
	// Transmission logger
	Logger *log.Logger

	// TCP connection
	mu   sync.Mutex
	conn net.Conn
	//closeTimer   *time.Timer
	//lastActivity time.Time
}

// Send sends data to server and ensures response length is greater than header length.
func (mb *tcpTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Establish a new connection if not connected
	if err = mb.connect(); err != nil {
		return
	}

	// Send data
	mb.logf("tcp: sending % x", aduRequest)
	if _, err = mb.conn.Write(aduRequest); err != nil {
		return
	}

	if _, err = io.ReadFull(mb.conn, aduResponse); err != nil {
		return
	}

	mb.logf("modbus: received % x\n", aduResponse)
	return
}

// Connect establishes a new connection to the address in Address.
// Connect and Close are exported so that multiple requests can be done with one session
func (mb *tcpTransporter) Connect() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	return mb.connect()
}

func (mb *tcpTransporter) connect() error {
	if mb.conn == nil {
		dialer := net.Dialer{Timeout: mb.Timeout}
		conn, err := dialer.Dial("tcp", mb.Address)
		if err != nil {
			return err
		}
		mb.conn = conn
	}
	return nil
}

/*
func (mb *tcpTransporter) startCloseTimer() {
	if mb.IdleTimeout <= 0 {
		return
	}
	if mb.closeTimer == nil {
		mb.closeTimer = time.AfterFunc(mb.IdleTimeout, mb.closeIdle)
	} else {
		mb.closeTimer.Reset(mb.IdleTimeout)
	}
}
*/

// Close closes current connection.
func (mb *tcpTransporter) Close() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	return mb.close()
}

/*
// flush flushes pending data in the connection,
// returns io.EOF if connection is closed.
func (mb *tcpTransporter) flush(b []byte) (err error) {
	if err = mb.conn.SetReadDeadline(time.Now()); err != nil {
		return
	}
	// Timeout setting will be reset when reading
	if _, err = mb.conn.Read(b); err != nil {
		// Ignore timeout error
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			err = nil
		}
	}
	return
}
*/
func (mb *tcpTransporter) logf(format string, v ...interface{}) {
	if mb.Logger != nil {
		mb.Logger.Printf(format, v...)
	}
}

// closeLocked closes current connection. Caller must hold the mutex before calling this method.
func (mb *tcpTransporter) close() (err error) {
	if mb.conn != nil {
		err = mb.conn.Close()
		mb.conn = nil
	}
	return
}
