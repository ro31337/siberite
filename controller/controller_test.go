package controller

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bogdanovich/siberite/repository"
	"github.com/stretchr/testify/assert"
)

var dir = "./test_data"
var name = "test"
var err error

type MockTCPConn struct {
	WriteBuffer bytes.Buffer
	ReadBuffer  bytes.Buffer
}

func NewMockTCPConn() *MockTCPConn {
	conn := &MockTCPConn{}
	return conn
}

func (conn *MockTCPConn) Read(b []byte) (int, error) {
	return conn.ReadBuffer.Read(b)
}

func (conn *MockTCPConn) Write(b []byte) (int, error) {
	return conn.WriteBuffer.Write(b)
}

func (conn *MockTCPConn) SetDeadline(t time.Time) error {
	return nil
}

func TestMain(m *testing.M) {
	_ = os.RemoveAll(dir)
	err = os.MkdirAll(dir, 0777)
	if err != nil {
		fmt.Println(err)
	}
	result := m.Run()
	err = os.RemoveAll(dir)
	os.Exit(result)
}

func Test_NewSession_FinishSession(t *testing.T) {
	repo, err := repository.Initialize(dir)
	defer repo.CloseAllQueues()
	assert.Nil(t, err)

	mockTCPConn := NewMockTCPConn()
	c := NewSession(mockTCPConn, repo)

	assert.Equal(t, uint64(1), repo.Stats.CurrentConnections)
	assert.Equal(t, uint64(1), repo.Stats.TotalConnections)

	c.FinishSession()
	assert.Equal(t, uint64(0), repo.Stats.CurrentConnections)
}

func Test_ReadFirstMessage(t *testing.T) {
	repo, err := repository.Initialize(dir)
	defer repo.CloseAllQueues()
	assert.Nil(t, err)

	mockTCPConn := NewMockTCPConn()
	controller := NewSession(mockTCPConn, repo)

	fmt.Fprintf(&mockTCPConn.ReadBuffer, "GET work\r\n")
	message, err := controller.ReadFirstMessage()
	assert.Nil(t, err)
	assert.Equal(t, "GET work\r\n", message)

	fmt.Fprintf(&mockTCPConn.ReadBuffer, "SET work 0 0 10\r\n0123456789\r\n")
	message, err = controller.ReadFirstMessage()
	assert.Nil(t, err)
	assert.Equal(t, "SET work 0 0 10\r\n", message)
}

func Test_UnknownCommand(t *testing.T) {
	repo, err := repository.Initialize(dir)
	defer repo.CloseAllQueues()
	assert.Nil(t, err)

	mockTCPConn := NewMockTCPConn()
	controller := NewSession(mockTCPConn, repo)

	err = controller.UnknownCommand()
	assert.Equal(t, "ERROR Unknown command", err.Error())
	assert.Equal(t, "ERROR Unknown command\r\n", mockTCPConn.WriteBuffer.String())

}

func Test_SendError(t *testing.T) {
	repo, err := repository.Initialize(dir)
	defer repo.CloseAllQueues()
	assert.Nil(t, err)

	mockTCPConn := NewMockTCPConn()
	controller := NewSession(mockTCPConn, repo)

	controller.SendError("Test error message")
	assert.Equal(t, "Test error message\r\n", mockTCPConn.WriteBuffer.String())
}
