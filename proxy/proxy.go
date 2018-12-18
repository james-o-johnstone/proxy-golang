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
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var errInvalidRequest = errors.New("Invalid request line")

var REQUEST_LINE_REGEX, _ = regexp.Compile("(GET|CONNECT) (?:http[s]*://)*(\\S+?)[:]*([\\d]+)*[/]* HTTP/1.1")

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

func handleConnection(conn net.Conn) (string, error) {
	p := make([]byte, 4096)
	_, err := conn.Read(p)
	if err != nil {
		return "", err
	}
	message := string(p)
	log.Print(message)
	request, err := parseMessage(message)
	if err != nil {
		return "", err
	}
	var response string
	if request.method == "CONNECT" {
		log.Print("Dialing")
//		upstreamConn, err := net.Dial(
//			"tcp", request.URI + ":" + request.port,
//		)
		upstreamConn, err := net.Dial(
			"tcp", request.URI + ":" + request.port, &tls.Config{},
		)
		if err != nil {
			return "", err
		}
		log.Print("Writing")
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		log.Print("Reading")
		p := make([]byte, 4096)
		_, err = conn.Read(p)
		log.Print("Read")
		if err != nil {
			return "", err
		}
		message = string(p)
		log.Printf("Writing upstream:\n%s", message)
		request, err = parseMessage(message)
		upstreamConn.Write(p)
		log.Print("Reading")
		buf := make([]byte, 10000)
		n, err := upstreamConn.Read(buf)
		log.Printf("Read %d bytes from upstream", n)
		conn.Write(buf)
		n, err = conn.Read(buf)
		log.Printf("Read %d bytes from downstream", n)
		upstreamConn.Write(buf)
		n, err = upstreamConn.Read(buf)
		return "", nil
	} else {
		response = makeUpstreamRequest(message, request)
	}
	log.Print(response)
	return response, nil
}

func runProxy(listenPort string) {
	listener, err := net.Listen("tcp", ":" + listenPort)
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Listening on:", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		response, err := handleConnection(conn)
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
			default:
				log.Fatal(err)
			}
		}
		if len(response) > 0 {
			conn.Write([]byte(response))
		}
	}
}

func main() {
	listenPort := os.Args[1]
	runProxy(listenPort)
}
