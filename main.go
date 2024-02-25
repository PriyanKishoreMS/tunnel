package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	vhost "github.com/inconshreveable/go-vhost"
	"github.com/progrium/qmux/golang/session"
)

func main() {
	var port = flag.String("p", "8080", "server port to use")
	var host = flag.String("h", "priyankishore.dev", "server hostname to use")
	var addr = flag.String("b", "0.0.0.0", "ip to bind [server only]")
	flag.Parse()

	//client code
	if flag.Arg(0) != "" {
		conn, err := net.Dial("tcp", net.JoinHostPort(*host, *port))
		fatal(err)
		client := httputil.NewClientConn(conn, bufio.NewReader(conn))
		req, err := http.NewRequest("GET", "/", nil)
		req.Host = net.JoinHostPort(*host, *port)
		fatal(err)
		client.Write(req)
		resp, _ := client.Read(req)
		fmt.Printf("port %s http available at:\n", flag.Arg(0))
		fmt.Printf("http://%s\n", resp.Header.Get("X-Public-Host"))
		c, _ := client.Hijack()
		sess := session.New(c)
		defer sess.Close()
		for {
			ch, err := sess.Accept()
			fatal(err)
			conn, err := net.Dial("tcp", "127.0.0.1:"+flag.Arg(0))
			fatal(err)
			go join(conn, ch)
		}
	}

	l, err := net.Listen("tcp", net.JoinHostPort(*addr, *port))
	fatal(err)
	defer l.Close()
	vmux, err := vhost.NewHTTPMuxer(l, 1*time.Second)
	fatal(err)

	go serve(vmux, *host, *port, "8081")

	log.Printf("gotunnel server [%s] ready!\n", *host)
	for {
		conn, err := vmux.NextError()
		fmt.Println(err)
		if conn != nil {
			conn.Close()
		}
	}
}

func join(a io.ReadWriteCloser, b io.ReadWriteCloser) {
	go io.Copy(b, a)
	io.Copy(a, b)
	a.Close()
	b.Close()
}

func newSubdomain() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	letters := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	r := make([]rune, 10)
	for i := range r {
		r[i] = letters[int(b[i])*len(letters)/256]
	}
	return string(r) + "."
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func serve(vmux *vhost.HTTPMuxer, host, port string, forwardPort string) {
	ml, err := vmux.Listen(net.JoinHostPort(host, port))
	fatal(err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardAddr := net.JoinHostPort(host, forwardPort)

		forwardConn, err := net.Dial("tcp", forwardAddr)
		if err != nil {
			log.Println("Failed to connect to forward port:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer forwardConn.Close()

		err = r.Write(forwardConn)
		if err != nil {
			log.Println("Failed to write request to forward port:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		resp, err := http.ReadResponse(bufio.NewReader(forwardConn), r)
		if err != nil {
			log.Println("Failed to read response from forward port:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})}
	srv.Serve(ml)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
