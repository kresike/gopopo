package main

import (
	"fmt"
	"log"
	"log/syslog"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/kresike/postfix"
	"github.com/sevlyar/go-daemon"
	"github.com/spf13/viper"
)

var rsw *(postfix.RatelimitSlidingWindow)
var logger *(log.Logger)

func main() {

	logger, err := syslog.NewLogger(syslog.LOG_INFO|syslog.LOG_DAEMON, 0)
	if err != nil {
		fmt.Println("Failed to open connection to syslog")
	}

	viper.SetDefault("ListenAddress", "localhost")
	viper.SetDefault("Port", "27091")
	viper.SetDefault("DomainWhiteList", "gopopo.wl")
	viper.SetDefault("DomainList", "gopopo.domains")
	viper.SetDefault("DefaultRate", "110")
	viper.SetDefault("Interval", "3660")

	viper.SetConfigName("gopopo")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc/gopopo")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("error parsing config file: %s", err))
	}

	host := viper.GetString("ListenAddress")
	port := viper.GetString("Port")

	wlfn := viper.GetString("DomainWhiteList")
	dlfn := viper.GetString("DomainList")

	wlmap := postfix.Load(wlfn)
	dommap := postfix.Load(dlfn)
	tmap := postfix.NewRatelimitTokenMap()
	tmap.SetLogger(logger)

	rsw = postfix.NewRatelimitSlidingWindow(wlmap, dommap, tmap)
	rsw.SetLogger(logger)

	rsw.SetDefaultLimit(viper.GetInt("DefaultRate"))

	rsw.SetInterval(viper.GetString("Interval"))

	defm := viper.GetString("DeferMessage")
	if defm != "" {
		rsw.SetDeferMessage(defm)
	}

	sock, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		logger.Println("Cannot create listening socket: " + err.Error())
		os.Exit(1)
	}
	defer sock.Close()

	cntxt := &daemon.Context{
		PidFileName: "/run/gopopo/gopopo.pid",
		PidFilePerm: 0644,
		WorkDir:     "./",
		Umask:       027,
		Args:        []string{"gopopo"},
	}

	d, err := cntxt.Reborn()
	if err != nil {
		logger.Fatal("Unable to run: ", err)
	}
	if d != nil {
		return
	}
	defer cntxt.Release()

	logger.Println("daemon started")

	logger.Println("Listening on " + host + ":" + port)
	for {
		conn, err := sock.Accept()
		if err != nil {
			logger.Println("Cannot accept client: " + err.Error())
			os.Exit(1)
		}
		go handleClient(conn)
	}
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func processMessage(buf []byte) string {
	parameters := strings.Fields(string(buf))
	result := postfix.NewPolicy()
	for _, param := range parameters {
		pv := strings.Split(param, "=")
		result.SetAttribute(pv[0], pv[1])
	}

	rcnt, err := strconv.Atoi(result.Attribute("recipient_count"))
	if err != nil {
		return "action=dunno\n\n"
	}

	sender := result.Attribute("sasl_username")
	if sender == "" {
		sender = result.Attribute("sender")
	}

	rsw.Report()
	return rsw.RateLimit(sender, rcnt)
}

func handleClient(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 0, 2048)
	lbuf := make([]byte, 256)

	for {
		rLen, err := conn.Read(lbuf)
		if err != nil {
			logger.Println("Error reading from client: " + err.Error())
			return
		}
		buf = append(buf, lbuf[:rLen]...)
		if equal(buf[len(buf)-2:], []byte("\n\n")) {
			break
		}
	}

	data := processMessage(buf)

	conn.Write([]byte(data))
}
