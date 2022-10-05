package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"time"
	"strconv"
	"strings"

	"github.com/LiamHaworth/go-tproxy"
)

var (
	// udpListener represents tje UDP
	// listening socket that will receive
	// UDP packets from TProxy
	udpListener *net.UDPConn

	listenPort int
	
	remoteIP string
	
	remotePort int
)

// main will initialize the TProxy
// handling application
func main() {
	listenPort, _ = strconv.Atoi(os.Args[1])
	remoteIPStr, remotePortStr, _ := net.SplitHostPort(os.Args[2])
	remoteIP = remoteIPStr
	remotePort, _ = strconv.Atoi(remotePortStr)

	log.Println("Starting UDP IPPort Forwarding TProxy")
	var err error

	log.Printf("Binding UDP TProxy listener to 0.0.0.0:%v", listenPort)
	udpListener, err = tproxy.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: listenPort})
	if err != nil {
		log.Fatalf("Encountered error while binding UDP listener: %s", err)
		return
	}

	defer udpListener.Close()
	go listenUDP()

	interruptListener := make(chan os.Signal)
	signal.Notify(interruptListener, os.Interrupt)
	<-interruptListener

	log.Println("TProxy listener closing")
	os.Exit(1)
}

// listenUDP runs in a routine to
// accept UDP connections and hand them
// off into their own routines for handling
func listenUDP() {
	for {
		buff := make([]byte, 1024)
		n, srcAddr, dstAddr, err := tproxy.ReadFromUDP(udpListener, buff)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("Temporary error while reading data: %s", netErr)
			}

			log.Fatalf("Unrecoverable error while reading data: %s", err)
			return
		}

		log.Printf("Accepting UDP connection from %s with destination of %s", srcAddr.String(), dstAddr.String())
		go handleUDPConn(buff[:n], srcAddr, dstAddr)
	}
}

// handleUDPConn will open a connection
// to the original destination pretending
// to be the client. It will when right
// the received data to the remote host
// and wait a few seconds for any possible
// response data
func handleUDPConn(data []byte, srcAddr, dstAddr *net.UDPAddr) {
	log.Printf("Accepting UDP connection from %s with destination of %s", srcAddr, dstAddr)

	localConn, err := tproxy.DialUDP("udp", dstAddr, srcAddr)
	if err != nil {
		log.Printf("Failed to connect to original UDP source [%s]: %s", srcAddr.String(), err)
		return
	}
	defer localConn.Close()

	IPv4Strs := strings.Split(remoteIP,".")
	IPv4Bytes := []byte{}
	for _, s := range IPv4Strs {
		IPInt, _:=strconv.Atoi(s)
		IPv4Bytes=append(IPv4Bytes,byte(IPInt))
	}
	dstAddr = &net.UDPAddr{
		IP:   net.IPv4(IPv4Bytes[0],IPv4Bytes[1],IPv4Bytes[2],IPv4Bytes[3]),
		Port: remotePort,
	}
	remoteConn, err := tproxy.DialUDP("udp", srcAddr, dstAddr)
	if err != nil {
		log.Printf("Failed to connect to original UDP destination [%s]: %s", dstAddr.String(), err)
		return
	}
	defer remoteConn.Close()

	bytesWritten, err := remoteConn.Write(data)
	if err != nil {
		log.Printf("Encountered error while writing to remote [%s]: %s", remoteConn.RemoteAddr(), err)
		return
	} else if bytesWritten < len(data) {
		log.Printf("Not all bytes [%d < %d] in buffer written to remote [%s]", bytesWritten, len(data), remoteConn.RemoteAddr())
		return
	}

	data = make([]byte, 1024)
	remoteConn.SetReadDeadline(time.Now().Add(2 * time.Second)) // Add deadline to ensure it doesn't block forever
	bytesRead, err := remoteConn.Read(data)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return
		}

		log.Printf("Encountered error while reading from remote [%s]: %s", remoteConn.RemoteAddr(), err)
		return
	}

	bytesWritten, err = localConn.Write(data)
	if err != nil {
		log.Printf("Encountered error while writing to local [%s]: %s", localConn.RemoteAddr(), err)
		return
	} else if bytesWritten < bytesRead {
		log.Printf("Not all bytes [%d < %d] in buffer written to locoal [%s]", bytesWritten, len(data), remoteConn.RemoteAddr())
		return
	}
}
