package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/tarm/serial"
	"github.com/urfave/cli"
)

type acceptResult struct {
	conn net.Conn
	err  error
}

type readResult struct {
	bytesRead int
	err       error
}

func main() {
	var port, baudrate int
	var serialPortName string

	app := cli.NewApp()
	app.Name = "Micro-Business TCP to Serial"
	app.Usage = "Listens on TCP ports and dumps the received packets on the serial port."
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:        "port, p",
			Usage:       "TCP port number to listen on, can't be negative or greater than 65535",
			Value:       9100,
			Destination: &port,
		},
		cli.StringFlag{
			Name:        "serialPortName, s",
			Usage:       "Serial port to redirect received packets from, examples: (COM1, COM2, /dev/ttyUSB0, ...)",
			Value:       "COM1",
			Destination: &serialPortName,
		},
		cli.IntFlag{
			Name:        "baudrate, b",
			Usage:       "Serial port baudrate",
			Value:       115200,
			Destination: &baudrate,
		},
	}
	app.Action = func(c *cli.Context) error {
		if port < 1 || port > 65535 {
			return fmt.Errorf("Invalid port number: %d", port)
		}

		listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			return fmt.Errorf("Can not listen on TCP port: %s", err)
		}

		defer close(listener)

		log.Printf("Listening on port %d", port)

		acceptChan := make(chan acceptResult)
		acceptMore := make(chan bool)

		go acceptFunc(listener, acceptChan, acceptMore)

		ip2serialBuffer := make([]byte, 1024)
		ipReadChan := make(chan readResult)

		// Things that belong to the current connection
		var currentConnection net.Conn
		var currentReadMore chan bool
		var connectionError, serialError error
		var serialPort *serial.Port

		for {
			select {
			case acceptResult := <-acceptChan:
				if acceptResult.err != nil {
					log.Printf("Failed to accept the connection: %s", acceptResult.err)
				} else {
					currentConnection = acceptResult.conn
					currentReadMore = make(chan bool)
					serialConfig := serial.Config{Name: serialPortName, Baud: baudrate}

					if serialPort, serialError = serial.OpenPort(&serialConfig); serialError != nil {
						log.Printf("Failed to open serial port: %s\n", serialError)
					} else {
						go readProc(currentConnection, ip2serialBuffer, ipReadChan, currentReadMore)
					}
				}

			case readResult := <-ipReadChan:
				if readResult.err != nil {
					log.Fatalf("Error reading from connection: %s", readResult.err)
					connectionError = readResult.err
				} else {
					if _, serialError = serialPort.Write(ip2serialBuffer[0:readResult.bytesRead]); serialError != nil {
						log.Fatalf("Error writing to serial port: %s", err)
					} else {
						currentReadMore <- true
					}
				}
			}

			if currentConnection != nil && (connectionError != nil || serialError != nil) {
				if currentConnection != nil {
					log.Println("Closing current connection...")

					if err := currentConnection.Close(); err != nil {
						log.Printf("Failed to close the current connection: %s", err)
					} else {
						log.Println("Current connection successfully closed.")
					}
				}

				currentConnection = nil
				connectionError = nil
				acceptMore <- true
			}
		}
	}

	err := app.Run(os.Args)

	if err != nil {
		log.Fatal(err)
	}
}

func acceptFunc(listener net.Listener, acceptChann chan acceptResult, acceptMore chan bool) {
	for {
		log.Println("Waiting for connection...")

		conn, err := listener.Accept()

		log.Println("Connection accepted")

		acceptChann <- acceptResult{conn: conn, err: err}

		if _, ok := <-acceptMore; !ok {
			return
		}
	}
}

func readProc(src io.Reader, buffer []byte, result chan readResult, readMore chan bool) {
	for {
		bytesRead, err := src.Read(buffer)

		result <- readResult{bytesRead: bytesRead, err: err}

		if _, ok := <-readMore; !ok {
			return
		}
	}
}

func close(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Fatal(err)
	}
}
