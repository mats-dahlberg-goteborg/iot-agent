package decoder

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type MessageDecoderFunc func(context.Context, []byte, func(context.Context, []byte) error ) error

func DefaultDecoder(ctx context.Context, msg []byte, fn func(context.Context, []byte) error) error {
	err := fn(ctx, msg)
	return err
}

func SenlabTBasicDecoder(ctx context.Context, msg []byte, fn func(context.Context, []byte) error ) error {

	dm := []struct {
		DevEUI    string `json:"devEui"`
		Payload   string `json:"payload"`
		Timestamp string `json:"timestamp"`
	}{}

	if err := json.Unmarshal(msg, &dm); err != nil {
		return err
	}

	b, err := hex.DecodeString(dm[0].Payload)
	if err != nil {
		return err
	}

	// | ID(1) | BatteryLevel(1) | Internal(n) | Temp(2)
	// | ID(1) | BatteryLevel(1) | Internal(n) | Temp(2) | Temp(2)
	if len(b) < 4 {
		return errors.New("invalid payload")
	}

	var p payload
	for _, d := range dm {

		err = decodePayload(b, &p)
		if err != nil {
			return err
		}

		result := struct {
			DevEUI       string
			Id           int
			BatteryLevel int
			Temperature  float32
			Timestamp    string
		}{
			d.DevEUI,
			p.ID,
			p.BatteryLevel,
			p.Temperature,
			d.Timestamp,
		}

		r, err := json.Marshal(&result)
		if err != nil {
			return err
		}

		err = fn(ctx, r)
		if err != nil {
			return err
		}		
	}

	return err
}

type payload struct {
	ID           int
	BatteryLevel int
	Temperature  float32
}

func decodePayload(b []byte, p *payload) error {
	id := int(b[0])
	if id == 1 {
		err := singleProbe(b, p)
		if err != nil {
			return err
		}
	}
	if id == 12 {
		err := dualProbe(b, p)
		if err != nil {
			return err
		}
	}

	// these values must be ignored since they are sensor reading errors
	if p.Temperature == -46.75 || p.Temperature == 85 {
		return errors.New("sensor reading error")
	}

	return nil
}

func singleProbe(b []byte, p *payload) error {
	var temp int16
	err := binary.Read(bytes.NewReader(b[len(b)-2:]), binary.BigEndian, &temp)
	if err != nil {
		return err
	}

	p.ID = int(b[0])
	p.BatteryLevel = (int(b[1]) / 254) * 100
	p.Temperature = float32(temp) / 16.0

	return nil
}

func dualProbe(b []byte, p *payload) error {
	return errors.New("unsupported payload")
}
