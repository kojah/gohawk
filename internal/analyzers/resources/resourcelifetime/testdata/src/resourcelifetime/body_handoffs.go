package resourcelifetime

import (
	"io"
	"net/http"
)

// The Body reference carries the response's obligation through value copies.
// Sending transfers uncertain ownership; it does not establish cleanup.
type bodyPacket struct {
	body   io.ReadCloser
	data   []byte
	status string
}

func sendBodyPacket(url string, ch chan bodyPacket) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	ch <- bodyPacket{body: response.Body}
	return nil
}

func discardBodyPacket(url string) error {
	response, err := http.Get(url) // want "owned resource from http.Get is not released on every return path"
	if err != nil {
		return err
	}
	packet := bodyPacket{body: response.Body}
	_ = packet
	return nil
}

func sendResponseStatus(url string, ch chan bodyPacket) error {
	response, err := http.Get(url) // want "owned resource from http.Get is not released on every return path"
	if err != nil {
		return err
	}
	ch <- bodyPacket{status: response.Status}
	return nil
}

func sendResponseBytes(url string, ch chan bodyPacket) error {
	response, err := http.Get(url) // want "owned resource from http.Get is not released on every return path"
	if err != nil {
		return err
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	ch <- bodyPacket{data: data}
	return nil
}

func selectBodyPacket(url string, ch chan bodyPacket) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	select {
	case ch <- bodyPacket{body: response.Body}:
	default:
	}
	return nil
}

func selectStatusPacket(url string, ch chan bodyPacket) error {
	response, err := http.Get(url) // want "owned resource from http.Get is not released on every return path"
	if err != nil {
		return err
	}
	select {
	case ch <- bodyPacket{status: response.Status}:
	default:
	}
	return nil
}
