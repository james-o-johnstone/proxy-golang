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

var REQUEST_LINE_REGEX, _ = regexp.Compile("GET (?http[s]*://)*(\\S+?)[:]*([\\d]+)*[/]* HTTP/1.1")

func isValidHTTPRequest(requestLine string) bool {
	if !REQUEST_LINE_REGEX.MatchString(requestLine) {
		return false
	}
	return true
}

func parseMessage(request string) (map[string]string, string, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(request))
	scanner.Scan()
	requestLine := scanner.Text()
	if !isValidHTTPRequest(requestLine) {
		return map[string]string{}, "", "", errInvalidRequest
	}

	matches := REQUEST_LINE_REGEX.FindStringSubmatch(requestLine)
	requestURI := matches[1]
	port := matches[2]
	fmt.Printf("Request URI: %s\n", requestURI)
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
	return headers, requestURI, port, nil
}

func makeUpstreamRequest(message string, requestURI string, port string) string {
	if port == "" {
		port = "80"
	}
	fmt.Println(port)
	conn, err := net.Dial("tcp", requestURI + ":" + port)
	if err != nil {
		log.Print(err)
		return ""
	}
	conn.Write([]byte(message))
	p := make([]byte, 4096)
	response, err := conn.Read(p)
	if err != nil {
		log.Print(err)
		return ""
	}
	return string(response)
}

func handleConnection(conn net.Conn) error {
	p := make([]byte, 4096)
	_, err := conn.Read(p)
	if err != nil {
		log.Fatal(err)
	}
	message := string(p)
	log.Print(message)
	_, requestURI, port, err := parseMessage(message)
	if err != nil {
		return err
	}
	response := makeUpstreamRequest(message, requestURI, port)
	log.Print(response)
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
		if err != nil {
			log.Print(err)
			continue
		}
		err = handleConnection(conn)
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
		conn.Close()
	}
}

func main() {
	listenPort := os.Args[1]
	runProxy(listenPort)
}
