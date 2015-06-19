package envelopewriter

import "github.com/cloudfoundry/sonde-go/events"

type EnvelopeWriter interface {
	Write(event *events.Envelope) error
}