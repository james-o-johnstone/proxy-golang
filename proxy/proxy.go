package main

//build a basic web proxy capable of accepting HTTP requests, making requests from remote servers, caching results, and returning data to a client.

// establish socket connection for listening to incoing conns
// read data from client and check for properly formatted http req
// invalid req should have error code

// parse URL from HTTP req - host, port and path

// check if the object is already cached - return from cache

// make connection to requested host using remote port or default of 80
// send http request received from client to the remote server

// cache object to disk after downloading, dont cache if marked no-cache or private (see RFC)
// only cache if returned with status code 200

// send response to client via the socket, close connection

// browser e.g. firefox might send multple requests to get images -need threading in that case

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var errInvalidRequest = errors.New("Invalid request line")

var REQUEST_LINE_REGEX, _ = regexp.Compile(`(GET|CONNECT|POST) (?:http[s]*://)*(\S+?)(?:[:]([\d]+))*[/]* HTTP/1.1`)

type Request struct {
	headers map[string]string
	method string
	port string
	URI string
}

func isValidHTTPRequest(requestLine string) bool {
	if !REQUEST_LINE_REGEX.MatchString(requestLine) {
		return false
	}
	return true
}

func parseMessage(message string) (Request, error) {
	scanner := bufio.NewScanner(strings.NewReader(message))
	scanner.Scan()
	requestLine := scanner.Text()
	if !isValidHTTPRequest(requestLine) {
		return Request{}, errInvalidRequest
	}

	matches := REQUEST_LINE_REGEX.FindStringSubmatch(requestLine)
	method := matches[1]
	URI := matches[2]
	port := matches[3]
	fmt.Printf("Method: %s\n", method)
	fmt.Printf("Request URI: %s\n", URI)
	fmt.Printf("Port: %s\n", port)

	headers := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		header := strings.Split(line, ": ")
		headers[header[0]] = header[1]
	}
	return Request{method: method, URI: URI, headers: headers, port:port}, nil
}

func makeUpstreamRequest(message string, request Request) string {
	if request.port == "" {
		request.port = "80"
	}
	fmt.Println(request.port)
	fmt.Println(request.URI + ":" + request.port)
//	dialer := net.Dialer{KeepAlive: headers['KeepAlive']
	conn, err := tls.Dial("tcp", request.URI + ":" + request.port, &tls.Config{})
	if err != nil {
		log.Print(err)
		return ""
	}
	log.Printf("Writing message to socket: %s\n", message)
	conn.Write([]byte(message))
	p := make([]byte, 4096)
	_, err = conn.Read(p)
	log.Println("Reading message from socket: ", string(p))
	if err != nil {
		log.Print(err)
		return ""
	}
	return string(p)
}

func read(conn net.Conn) ([]byte, error) {
	var rBuffer bytes.Buffer
	nReadTot := 0
	connReader := bufio.NewReader(conn)
	rTempBuf := make([]byte, 4096)
//	size, err := connReader.ReadByte()
//	log.Printf("Packet size: %d\n", size)
//	if err != nil {
//		if err != io.EOF{
//			log.Fatal(err)
//		}
//		log.Print("EOF Reached")
//		return rBuffer.Bytes()
//	}

	n, err := connReader.Read(rTempBuf)
	if err != nil {
		if err != io.EOF{
			log.Print(err)
			return make([]byte, 0), err
		}
		log.Print("EOF Reached")
		return rTempBuf, nil
	}
//		n, err := conn.Read(rTempBuf)
//		if err != nil {
//			if err != io.EOF {
//				log.Fatal(err)
//			}
//			log.Print("EOF reached")
//			break
//		}
	log.Printf("Read: %d bytes\n", n)
	rBuffer.Write(rTempBuf)
	nReadTot += n
	log.Printf("Read Total: %d bytes\n", nReadTot)
	return rBuffer.Bytes(), nil
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func handleConnection(conn net.Conn) error {
	downstreamBytesRead := make([]byte, 4096)
	_, err := conn.Read(downstreamBytesRead)
	if err != nil {
		return err
	}
	message := string(downstreamBytesRead)
	log.Print(message)
	request, err := parseMessage(message)
	if err != nil {
		return err
	}
	var upstreamConn net.Conn
	if request.method == "CONNECT" {
		// open socket to create tunnel to upstream then send 200 OK back to downstream
		log.Printf("Dialing: %s\n", request.URI + ":" + request.port)
		upstreamConn, err = net.Dial(
			"tcp", request.URI + ":" + request.port,
		)
		if err != nil {
			return err
		}
//		upstreamConn, err := tls.Dial(
//			"tcp", request.URI + ":" + request.port, &tls.Config{},
//		)
		log.Print("Writing downstream CONNECT OK")
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
//		go transfer(upstreamConn, conn)
//		go transfer(conn, upstreamConn)
	} else {
		// if not CONNECT then just open socket and write the initial client request
		upstreamConn, err = net.Dial(
			"tcp", request.URI + ":" + request.port,
		)
		if err != nil {
			return err
		}
		_, err = upstreamConn.Write(downstreamBytesRead)
		upstreamBytesRead := make([]byte, 4096)
		_, err = upstreamConn.Read(upstreamBytesRead)
		conn.Write(upstreamBytesRead)
	}

	// pass data to/from the client until comms have stopped
	for {
		log.Print("Reading from downstream")
		readDownstream, err := read(conn)
		if err != nil {
			break
		}
		log.Print("Writing to upstream")
		upstreamConn.Write(readDownstream)
		log.Print("Reading from upstream")
		readUpstream, err := read(upstreamConn)
		if err != nil {
			break
		}
		log.Print("Writing to downstream")
		conn.Write(readUpstream)
	}
	return nil
}

func runProxy(listenPort string) {
	listener, err := net.Listen("tcp", ":" + listenPort)
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Listening on:", listenPort)

	for {
		conn, err := listener.Accept()
		log.Print("Accepting new connection")
		if err != nil {
			log.Print(err)
			continue
		}
		go func() {
			err := handleConnection(conn)
			if err != nil {
				switch err {
				case errInvalidRequest:
					response := fmt.Sprintf("HTTP/1.1 400 Bad Request\n" +
					"Date: %s\n" +
					"Content-Length: 0\n" +
					"Content-Type: text/html; charset=UTF-8\n" +
					"Connection: Closed\r\n",
					time.Now().UTC().Format(http.TimeFormat))
					log.Printf("Response:\n%s", response)
					conn.Write([]byte(response))
					conn.Close()
				default:
					log.Print(err)
				}
			}
			conn.Close()
		}()
	}
}

func main() {
	listenPort := os.Args[1]
	runProxy(listenPort)
}
