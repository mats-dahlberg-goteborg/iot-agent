package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/diwise/iot-agent/internal/pkg/application/conversion"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/farshidtz/senml/v2"
)

func (a *api) incomingSchneiderMessageHandler(ctx context.Context) http.HandlerFunc {
	logger := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx, span := tracer.Start(r.Context(), "incoming-schneider-message")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		_, _, log := o11y.AddTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		msg, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		log.Debug().Str("body", string(msg)).Msg("starting to process message")

		dataList := []Data{}

		err = json.Unmarshal(msg, &dataList)
		if err != nil {
			log.Error().Err(err).Msg("failed to handle message")

			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		for _, object := range dataList {
			name, err := trimName(object.Name)
			if err != nil {
				log.Error().Err(err).Msg("failed to trim name")

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}

			value, err := strconv.ParseFloat(object.Value, 64)
			if err != nil {
				log.Error().Err(err).Msg("failed to parse value")

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}

			basename := ""
			unit := ""

			if object.Unit == "°C" {
				basename = conversion.TemperatureURN
				unit = senml.UnitCelsius
			} else if object.Unit == "Wh" {
				basename = conversion.EnergyURN
				unit = senml.UnitJoule
				value = value * 3600
			} else if object.Unit == "W" {
				basename = conversion.PowerURN
				unit = senml.UnitWatt
			}

			decorators := []conversion.SenMLDecoratorFunc{
				conversion.ValueWithUnit("5700", unit, value),
			}

			pack := conversion.NewSenMLPack(name, basename, time.Now().UTC(), decorators...)
			b, _ := json.Marshal(pack)
			fmt.Println(string(b))

			url := a.forwardingEndpoint + "/lwm2m"

			resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
			if err != nil {
				log.Error().Err(err).Msg("failed to post senml pack")

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}

			if resp.StatusCode != http.StatusCreated {
				log.Error().Err(err).Msgf("request failed, expected status code %d but got status code %d ", http.StatusCreated, resp.StatusCode)

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

func trimName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	prefix := "/Enterprise Server Mitthem/IoT-gränssnitt/MQTT-klient/!UC_Framåt/"
	suffix := "/Value"

	name = strings.TrimPrefix(name, prefix)
	name = strings.TrimSuffix(name, suffix)

	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, "Å", "A")
	name = strings.ToLower(name)

	return name, nil
}

type DTO struct {
	Name  string
	Value float64
	Unit  string
}

type Data struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}