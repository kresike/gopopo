package main

import (
	"fmt"
	"log"
	"log/syslog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"strconv"
	"strings"
	"gopopo/postfix"

	"github.com/spf13/viper"
)

var rsw *(postfix.RatelimitSlidingWindow)
var logger *(log.Logger)

func main() {

	debug.SetTraceback("crash")

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

	wlmap := postfix.Load(wlfn, logger)
	dommap := postfix.Load(dlfn, logger)
	tmap := postfix.NewRatelimitTokenMap()
	tmap.SetLogger(logger)

	rsw = postfix.NewRatelimitSlidingWindow(wlmap, dommap, tmap)
	rsw.SetLogger(logger)

	cacheFileName := viper.GetString("CacheFileName")

	if cacheFileName != "" {
		if _, err := os.Stat(cacheFileName); err == nil {
			logger.Println("Found previously saved token cache, loading ...")
			rsw.LoadTokens(cacheFileName)
			rsw.Report()
		} else if os.IsNotExist(err) {
			logger.Println("No cache file found, not loading previous tokens")
		} else {
			panic(fmt.Errorf("error checking for cache file: %s", err))
		}
	}

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
	logger.Println("Listening on " + host + ":" + port)

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			sig := <-sigs
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {

				logger.Println("Saving tokens to ",cacheFileName)
				rsw.SaveTokens(cacheFileName)
				os.Exit(0)

			} else {
				logger.Println("Received signal",sig,"reloading data files")
				wmap := postfix.Load(wlfn, logger)
				dmap := postfix.Load(dlfn, logger)
				rsw.SetWhiteList(wmap)
				rsw.SetDomainList(dmap)
			}
		}
	}()

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
		if len(pv) < 2 {
			logger.Println("ERROR: got a bad parameter from postfix in buffer ->", buf, "Trying to continue anyway.");
			result.SetAttribute(param,"");
		} else {
			result.SetAttribute(pv[0], pv[1])
		}
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
