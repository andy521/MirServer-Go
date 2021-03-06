package gameserver

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pangliang/MirServer-Go/loginserver"
	"github.com/pangliang/MirServer-Go/mockclient"
	"github.com/pangliang/MirServer-Go/protocol"
	"github.com/pangliang/MirServer-Go/tools"
)

const (
	LOGIN_SERVER_ADDRESS = "127.0.0.1:7000"
	GAME_SERVER_ADDRESS  = "127.0.0.1:7400"
	DB_DRIVER            = "sqlite3"
)

func initTestDB(dbfile string) (err error) {
	tools.CreateDatabase(loginserver.Tables, DB_DRIVER, dbfile, true)
	tools.CreateDatabase(Tables, DB_DRIVER, dbfile, true)

	db, err := gorm.Open(DB_DRIVER, dbfile)
	defer db.Close()
	if err != nil {
		log.Fatalf("open database error : %s", err)
	}
	db.Create(&loginserver.User{
		Name:     "pangliang",
		Password: "pwd",
	})
	db.Create(&loginserver.User{
		Name:     "11",
		Password: "11",
	})
	db.Create(&loginserver.ServerInfo{
		Id:              1,
		GameServerIp:    "127.0.0.1",
		GameServerPort:  7400,
		LoginServerIp:   "127.0.0.1",
		LoginServerPort: 7000,
		Name:            "test1",
	})

	db.Create(&loginserver.ServerInfo{
		Id:              2,
		GameServerIp:    "192.168.0.166",
		GameServerPort:  7400,
		LoginServerIp:   "192.168.0.166",
		LoginServerPort: 7000,
		Name:            "test2",
	})

	return
}

var mockClient *mockclient.MockClient
var cert int

func TestMain(m *testing.M) {
	tmpfile, err := ioutil.TempFile("", "mir2.db")
	defer os.Remove(tmpfile.Name())

	err = initTestDB(tmpfile.Name())
	if err != nil {
		log.Fatal(err)
	}

	loginServer := loginserver.New(&loginserver.Option{
		IsTest:         true,
		Address:        LOGIN_SERVER_ADDRESS,
		DataSourceName: tmpfile.Name(),
		DriverName:     DB_DRIVER,
	})
	loginServer.Main()

	gameServer := New(&Option{
		IsTest:         true,
		Address:        GAME_SERVER_ADDRESS,
		DataSourceName: tmpfile.Name(),
		DriverName:     DB_DRIVER,
	})
	gameServer.Main()

	retCode := m.Run()

	if mockClient != nil {
		mockClient.Close()
	}

	loginServer.Exit()
	gameServer.Exit()
	os.Exit(retCode)
}

func sendAndCheck(client *mockclient.MockClient, request *protocol.Packet, expect *protocol.Packet) (err error) {
	client.Send(request)
	resp, err := client.Read()
	if err != nil {
		return
	}
	if *resp != *expect {
		return errors.New(fmt.Sprint(expect, resp))
	}
	return nil
}

func TestLogin(t *testing.T) {
	loginClient, err := mockclient.New(LOGIN_SERVER_ADDRESS)
	defer loginClient.Close()
	if err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(loginClient,
		&protocol.Packet{protocol.PacketHeader{0, loginserver.CM_IDPASSWORD, 0, 0, 0}, "pangliang/pwd"},
		&protocol.Packet{protocol.PacketHeader{0, loginserver.SM_PASSOK_SELECTSERVER, 0, 0, 2}, "test1/1/test2/2/"},
	); err != nil {
		t.Fatal(err)
	}

	loginClient.Send(&protocol.Packet{protocol.PacketHeader{0, loginserver.CM_SELECTSERVER, 0, 0, 0}, "test1"})
	resp, err := loginClient.Read()
	if err != nil {
		t.Fatal(fmt.Sprint(err))
	}
	params := resp.Params()
	if (len(params) != 3 || params[0] != "127.0.0.1" || params[1] != "7400") ||
		resp.Header.Protocol != loginserver.SM_SELECTSERVER_OK {
		t.Fatal(fmt.Sprint(resp))
	}
	cert, _ = strconv.Atoi(params[2])

	mockClient, err = mockclient.New(GAME_SERVER_ADDRESS)
	if err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_QUERYCHR, 0, 0, 0}, fmt.Sprintf("pangliang/%d", cert)},
		&protocol.Packet{protocol.PacketHeader{0, SM_QUERYCHR, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}
}

func TestFailLogin(t *testing.T) {
	newClient, err := mockclient.New(GAME_SERVER_ADDRESS)
	if err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(newClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_QUERYCHR, 0, 0, 0}, "pangliang"},
		&protocol.Packet{protocol.PacketHeader{1, SM_QUERYCHR_FAIL, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(newClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_QUERYCHR, 0, 0, 0}, "pangliang1/1000"},
		&protocol.Packet{protocol.PacketHeader{2, SM_QUERYCHR_FAIL, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(newClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_QUERYCHR, 0, 0, 0}, "pangliang/1000"},
		&protocol.Packet{protocol.PacketHeader{3, SM_QUERYCHR_FAIL, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	newClient.Send(&protocol.Packet{protocol.PacketHeader{0, CM_NEWCHR, 0, 0, 0}, "pangliang/player1/1/1/1/"})
	resp, err := newClient.Read()
	if err == nil || err.Error() != "EOF" {
		t.Fatal(fmt.Sprint(err, resp))
	}
}

func TestCreateDeletePlayer(t *testing.T) {
	if mockClient == nil {
		t.Fatal("client is not")
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_NEWCHR, 0, 0, 0}, "pangliang/player1/3/2/1/"},
		&protocol.Packet{protocol.PacketHeader{0, SM_NEWCHR_SUCCESS, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_NEWCHR, 0, 0, 0}, "pangliang/player2/1/1/2/"},
		&protocol.Packet{protocol.PacketHeader{0, SM_NEWCHR_SUCCESS, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_QUERYCHR, 0, 0, 0}, fmt.Sprintf("pangliang/%d", cert)},
		&protocol.Packet{protocol.PacketHeader{2, SM_QUERYCHR, 0, 0, 0}, "player1/2/3/1/1/player2/1/1/1/2/"},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_NEWCHR, 0, 0, 0}, "pangliang/player1/1/1/1/"},
		&protocol.Packet{protocol.PacketHeader{2, SM_NEWCHR_FAIL, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_DELCHR, 0, 0, 0}, "player1"},
		&protocol.Packet{protocol.PacketHeader{0, SM_DELCHR_SUCCESS, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}

	if err := sendAndCheck(mockClient,
		&protocol.Packet{protocol.PacketHeader{0, CM_DELCHR, 0, 0, 0}, "player1"},
		&protocol.Packet{protocol.PacketHeader{2, SM_DELCHR_FAIL, 0, 0, 0}, ""},
	); err != nil {
		t.Fatal(err)
	}
}
