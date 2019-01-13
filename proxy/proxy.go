package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var errInvalidRequest = errors.New("Invalid request line")

var REQUEST_LINE_REGEX, _ = regexp.Compile(`(GET|CONNECT|POST) (?:(http[s]*)://)*(\S+?)(?:[:]([\d]+))*[/]* HTTP/1.1`)

type Request struct {
	headers map[string]string
	method  string
	port    string
	URI     string
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
	proto := matches[2]
	URI := matches[3]
	port := matches[4]
	if port == "" && proto == "https" {
		port = "443"
	} else if port == "" && proto == "http" {
		port = "80"
	}
	log.Printf("Proto: %s, Method: %s, Request URI: %s, Port: %s", proto, method, URI, port)

	headers := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		header := strings.Split(line, ": ")
		headers[header[0]] = header[1]
	}
	return Request{method: method, URI: URI, headers: headers, port: port}, nil
}

func read(conn net.Conn) (string, error) {
	connReader := bufio.NewReader(conn)
	var HTTPReqBuilder strings.Builder
	var HTTPReq string
	for {
		tempBuf := make([]byte, 100)
		_, err := connReader.Read(tempBuf)
		if err != nil {
			if err != io.EOF {
				log.Print(err)
				return "", err
			}
		}
		HTTPReqBuilder.WriteString(strings.TrimRight(string(tempBuf), "\x00"))
		HTTPReq = HTTPReqBuilder.String()
		if len(HTTPReq) >= 4 && HTTPReq[len(HTTPReq)-4:] == "\r\n\r\n" {
			return HTTPReq, nil
		}
	}
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func proxy(upstream net.Conn, client net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		transfer(upstream, client)
		wg.Done()
	}()
	go func() {
		transfer(client, upstream)
		wg.Done()
	}()
	wg.Wait()
}

func handleConnection(clientConn net.Conn) error {
	clientRawRequest, err := read(clientConn)
	if err != nil {
		return err
	}
	log.Printf("Read from client:\n%s", clientRawRequest)
	request, err := parseMessage(clientRawRequest)
	if err != nil {
		return err
	}

	var serverConn net.Conn
	log.Printf("Dialing: %s\n", request.URI+":"+request.port)
	serverConn, err = net.Dial(
		"tcp", request.URI+":"+request.port,
	)
	if err != nil {
		return err
	}
	if request.method == "CONNECT" {
		log.Print("Writing CONNECT OK to client")
		clientConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

		log.Print("Proxying TCP byte stream")
		proxy(serverConn, clientConn)
	} else {
		serverConn.Write([]byte(clientRawRequest))
		rawResponse, err := read(serverConn)
		if err != nil {
			return err
		}
		log.Printf("Read from server:\n%s", string(rawResponse))
		clientConn.Write([]byte(rawResponse))
	}
	log.Printf("Finished handling connection to: %s", request.URI+":"+request.port)
	return nil
}

func runProxy(listenPort string) {
	listener, err := net.Listen("tcp", ":"+listenPort)
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Listening to port: ", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		log.Print("Accepted new connection")
		go func(c net.Conn) {
			err := handleConnection(c)
			if err != nil {
				switch err {
				case errInvalidRequest:
					response := fmt.Sprintf("HTTP/1.1 400 Bad Request\n"+
						"Date: %s\n"+
						"Content-Length: 0\n"+
						"Content-Type: text/html; charset=UTF-8\n"+
						"Connection: Closed\r\n",
						time.Now().UTC().Format(http.TimeFormat))
					log.Printf("Response:\n%s", response)
					c.Write([]byte(response))
				default:
					log.Print(err)
				}
			}
			log.Print("Closing connection")
			c.Close()
		}(conn)
	}
}

func main() {
	listenPort := os.Args[1]
	runProxy(listenPort)
}
