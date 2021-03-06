package syslog

import (
	"doppler/sinks/retrystrategy"
	"doppler/sinks/syslogwriter"
	"fmt"
	"sync"
	"time"

	"doppler/sinks"

	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
)

type SyslogSink struct {
	logger                 *gosteno.Logger
	appId                  string
	drainUrl               string
	sentMessageCount       *uint64
	sentByteCount          *uint64
	messageDrainBufferSize uint
	listenerChannel        chan *events.Envelope
	syslogWriter           syslogwriter.Writer
	handleSendError        func(errorMessage, appId, drainUrl string)
	disconnectChannel      chan struct{}
	dropsondeOrigin        string
	disconnectOnce         sync.Once
}

func NewSyslogSink(appId string, drainUrl string, givenLogger *gosteno.Logger, messageDrainBufferSize uint, syslogWriter syslogwriter.Writer, errorHandler func(string, string, string), dropsondeOrigin string) *SyslogSink {
	givenLogger.Debugf("Syslog Sink %s: Created for appId [%s]", drainUrl, appId)
	return &SyslogSink{
		appId:                  appId,
		drainUrl:               drainUrl,
		logger:                 givenLogger,
		messageDrainBufferSize: messageDrainBufferSize,
		syslogWriter:           syslogWriter,
		handleSendError:        errorHandler,
		disconnectChannel:      make(chan struct{}),
		dropsondeOrigin:        dropsondeOrigin,
	}
}

func (s *SyslogSink) Run(inputChan <-chan *events.Envelope) {
	s.logger.Infof("Syslog Sink %s: Running.", s.drainUrl)
	defer s.logger.Errorf("Syslog Sink %s: Stopped.", s.drainUrl)

	backoffStrategy := retrystrategy.NewExponentialRetryStrategy()
	numberOfTries := 0
	filteredChan := make(chan *events.Envelope)

	go func() {
		defer close(filteredChan)

		for {
			select {
			case v, ok := <-inputChan:
				if !ok {
					return
				}

				if v.GetEventType() != events.Envelope_LogMessage {
					continue
				}

				filteredChan <- v
			case <-s.disconnectChannel:
				return
			}
		}
	}()

	buffer := sinks.RunTruncatingBuffer(filteredChan, s.messageDrainBufferSize, s.logger, s.dropsondeOrigin, s.Identifier())
	timer := time.NewTimer(backoffStrategy(numberOfTries))
	connected := false
	defer timer.Stop()
	defer s.syslogWriter.Close()

	s.logger.Debugf("Syslog Sink %s: Starting loop. Current backoff: %v", s.drainUrl, backoffStrategy(numberOfTries))
	for {
		if !connected {
			s.logger.Debugf("Syslog Sink %s: Not connected. Trying to connect.", s.drainUrl)
			err := s.syslogWriter.Connect()
			if err != nil {
				sleepDuration := backoffStrategy(numberOfTries)
				errorMsg := fmt.Sprintf("Syslog Sink %s: Error when dialing out. Backing off for %v. Err: %v", s.drainUrl, sleepDuration, err)

				s.handleSendError(errorMsg, s.appId, s.drainUrl)

				timer.Reset(sleepDuration)
				select {
				case <-s.disconnectChannel:
					return
				case <-timer.C:
				}

				numberOfTries++
				continue
			}

			s.logger.Infof("Syslog Sink %s: successfully connected.", s.drainUrl)
			connected = true
		}

		s.logger.Debugf("Syslog Sink %s: Waiting for activity\n", s.drainUrl)

		select {
		case <-s.disconnectChannel:
			return
		case messageEnvelope, ok := <-buffer.GetOutputChannel():
			if !ok {
				s.logger.Debugf("Syslog Sink %s: Closed listener channel detected. Closing.\n", s.drainUrl)
				return
			}

			// Some metrics will not be filter and can get to here (i.e.: TruncatingBuffer dropped message metrics)
			if messageEnvelope.GetEventType() == events.Envelope_LogMessage {
				err := s.sendLogMessage(messageEnvelope.GetLogMessage())
				if err == nil {
					numberOfTries = 0
					connected = true
				} else {
					s.logger.Debugf("Syslog Sink %s: Error when trying to send data to sink. Backing off. Err: %v\n", s.drainUrl, err)
					numberOfTries++
					connected = false
				}
			}
		}
	}
}

func (s *SyslogSink) Disconnect() {
	s.disconnectOnce.Do(func() { close(s.disconnectChannel) })
}

func (s *SyslogSink) Identifier() string {
	return s.drainUrl
}

func (s *SyslogSink) StreamId() string {
	return s.appId
}

func (s *SyslogSink) ShouldReceiveErrors() bool {
	return false
}

func (s *SyslogSink) sendLogMessage(logMessage *events.LogMessage) error {
	_, err := s.syslogWriter.Write(messagePriorityValue(logMessage), logMessage.GetMessage(), logMessage.GetSourceType(), logMessage.GetSourceInstance(), *logMessage.Timestamp)
	return err
}

func messagePriorityValue(msg *events.LogMessage) int {
	switch msg.GetMessageType() {
	case events.LogMessage_OUT:
		return 14
	case events.LogMessage_ERR:
		return 11
	default:
		return -1
	}
}
