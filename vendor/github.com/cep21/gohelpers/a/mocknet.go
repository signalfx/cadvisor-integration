package a

import (
	"github.com/stretchr/testify/mock"
	"time"
	"net"
)
type Conn struct {
	mock.Mock
}

func (m *Conn) Read(b []byte) (int, error) {
	ret := m.Called(b)

	r0 := ret.Get(0).(int)
	r1 := ret.Error(1)

	return r0, r1
}
func (m *Conn) Write(b []byte) (int, error) {
	ret := m.Called(b)

	r0 := ret.Get(0).(int)
	r1 := ret.Error(1)

	return r0, r1
}
func (m *Conn) Close() error {
	ret := m.Called()

	r0 := ret.Error(0)

	return r0
}
func (m *Conn) LocalAddr() net.Addr {
	ret := m.Called()

	r0 := ret.Get(0).(net.Addr)

	return r0
}
func (m *Conn) RemoteAddr() net.Addr {
	ret := m.Called()

	r0 := ret.Get(0).(net.Addr)

	return r0
}
func (m *Conn) SetDeadline(t time.Time) error {
	ret := m.Called(t)

	r0 := ret.Error(0)

	return r0
}
func (m *Conn) SetReadDeadline(t time.Time) error {
	ret := m.Called(t)

	r0 := ret.Error(0)

	return r0
}
func (m *Conn) SetWriteDeadline(t time.Time) error {
	ret := m.Called(t)

	r0 := ret.Error(0)

	return r0
}
