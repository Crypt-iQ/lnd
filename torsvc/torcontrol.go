package torsvc

import (
	"bufio"
	"fmt"
	"net"
	"net/textproto"
	"strings"
)

const (
	// addStr represents the Tor control port command that creates a new
	// hidden service.
	addStr  = "ADD_ONION"

	// authStr represents the Tor control port command that authenticates to
	// the control port.
	authStr = "AUTHENTICATE"

	// success is the Tor control port response code representing a
	// successful request.
	success = 250
)

// TorControl houses options for interacting with Tor's ControlPort. These
// options determine the hidden service creation configuration that LND will use
// when automatically creating hidden services.
type TorControl struct {
	conn     net.Conn
	reader   *textproto.Reader
	Password string
	Port     string
	TargPort string
	VirtPort string
	PrivKey  string
	Save     bool
}

// AuthWithPass authenticates via password to Tor's ControlPort.
//
// Note: AuthWithPass must be called even if the ControlPort has no
// authentication mechanism in place.
func (tc *TorControl) AuthWithPass() error {
	_, _, err := tc.sendCommand(authStr + " \"" + tc.Password + "\"\n")
	return err
}

// AddOnion creates a Tor v2 hidden service. This hidden service is available as
// long as the connection to Tor's ControlPort is kept open.
func (tc *TorControl) AddOnion() (string, error) {
	var command string

	// Use the lnd-generated private key to create a v2 hidden service.
	command = addStr + " RSA1024:" + tc.PrivKey

	// Add the VIRTPORT and TARGET ports to the command.
	command += " Port=" + tc.VirtPort + "," + tc.TargPort + "\n"

	// Send the command to Tor's ControlPort.
	_, message, err := tc.sendCommand(command)
	if err != nil {
		return "", err
	}

	// The response format we get for issuing a successful ADD_ONION command
	// is as follows:
	//
	// ------------------------------
	// 250-ServiceID=testonion1234567
	// 250-PrivateKey=RSA1024:[Blob Redacted]
	// 250 OK
	// ------------------------------
	//
	// When the ADD_ONION command DOES NOT request a fresh private key
	// and instead provides one, the 250-PrivateKey line is omitted from the
	// response. Since lnd generates its own private keys, the
	// 250-PrivateKey line will not be returned in the response. The
	// response will instead look like this:
	//
	// ------------------------------
	// 250-ServiceID=testonion1234567
	// 250 OK
	// ------------------------------

	// Next, we parse out the hidden service (testonion1234567 in the example
	// response above) from the response and return it.

	// First, we retrieve the index of the first "=" since it is between
	// "ServiceID" and the hidden service string.
	equalIndex := strings.Index(message, "=")
	if equalIndex == -1 {
		return "", fmt.Errorf("Could not retrieve hidden service")
	}

	// Next, we retrieve the index of the first "\n" which should be at
	// the end of the first response line.
	newLineIndex := strings.Index(message, "\n")
	if newLineIndex == -1 {
		return "", fmt.Errorf("Could not retrieve hidden service")
	}

	// message[equalIndex+1 : newLineIndex] gives us the hidden service string,
	// which is between the first "=" and the first "\n" in the response
	// string.
	return message[equalIndex+1 : newLineIndex], err
}

// Open opens the connection to Tor's ControlPort.
func (tc *TorControl) Open() error {
	var err error
	tc.conn, err = net.Dial("tcp", "localhost:"+tc.Port)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(tc.conn)
	tc.reader = textproto.NewReader(reader)

	return nil
}

// Close closes the connection to Tor's ControlPort.
func (tc *TorControl) Close() error {
	if err := tc.conn.Close(); err != nil {
		return err
	}
	tc.reader = nil
	return nil
}

// sendCommand sends a command for execution to Tor's ControlPort.
func (tc *TorControl) sendCommand(command string) (int, string, error) {
	// Write command to Tor's ControlPort.
	_, err := tc.conn.Write([]byte(command))
	if err != nil {
		return 0, "", fmt.Errorf("Writing to Tor's ControlPort failed: %s", err)
	}

	// ReadResponse supports multi-line responses whereas ReadCodeLine does not.
	// In some cases, the response will need to be parsed out from the message
	// variable.
	code, message, err := tc.reader.ReadResponse(success)
	if err != nil {
		return code, message, fmt.Errorf("Reading Tor's ControlPort "+
			"command status failed: %s", err)
	}

	return code, message, nil
}
