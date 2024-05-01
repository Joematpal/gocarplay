package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mzyy94/gocarplay/link"
	"github.com/mzyy94/gocarplay/protocol"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

var Connect = func(ctx context.Context) (io.Reader, io.Writer, error) {
	in, out, err := link.Connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	return in, out, nil
}

type ConnectFunc func(ctx context.Context) (io.Reader, io.Writer, error)

func (f ConnectFunc) Connect(ctx context.Context) (io.Reader, io.Writer, error) {
	return f(ctx)
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type Connector interface {
	Connect(context.Context) (io.Reader, io.Writer, error)
}

type Server struct {
	ctx              context.Context
	videoTrack       *webrtc.TrackLocalStaticSample
	audioDataChannel *webrtc.DataChannel
	size             *link.ScreenSize
	fps              int32
	logger           Logger
	connector        Connector
	in               io.Reader
	out              io.Writer
}

func NewServer(opts ...Option) (http.Handler, error) {
	s := &Server{
		ctx: context.Background(),
		fps: 25,
		connector: ConnectFunc(func(ctx context.Context) (io.Reader, io.Writer, error) {
			return Connect(ctx)
		}),
	}

	for _, opt := range opts {
		if err := opt.apply(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Server) Debug(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Debug(msg, args...)
	}
}

func (s *Server) Info(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Info(msg, args...)
	}
}

func (s *Server) Warn(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, args...)
	}
}

func (s *Server) Error(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Error(msg, args...)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.webRTCOfferHandler(w, r)
}

func (s *Server) webRTCOfferHandler(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "{\"error\": \"%s\"}", err.Error())
		return
	}

	answer, err := s.setupWebRTC(r.Context(), offer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{\"error\": \"%s\"}", err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&answer)
}

func (s *Server) setupWebRTC(ctx context.Context, offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	// todo make this listen from kill or term signal
	// how many times does this run?

	s.Debug("setup web rtc")

	var err error
	s.in, s.out, err = s.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}

	lnk, err := link.New(
		link.WithContext(context.Background()),
		link.WithDPI(160),
		link.WithFPS(s.fps),
		link.WithReader(s.in),
		link.WithWriter(s.out),
	)
	if err != nil {
		return nil, err
	}

	// WebRTC setup
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	mediaEngine := webrtc.MediaEngine{}

	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	stats, ok := pc.GetStats().GetConnectionStats(pc)
	if !ok {
		stats.ID = "unknown"
	}

	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		s.Info("state of %s: %s", stats.ID, connectionState.String())
	})

	// Create a video track
	videoCodec := webrtc.RTPCodecCapability{
		MimeType:     webrtc.MimeTypeH264,
		ClockRate:    90000,
		Channels:     0,
		SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
		RTCPFeedback: nil,
	}

	if s.videoTrack, err = webrtc.NewTrackLocalStaticSample(videoCodec, "video", "video"); err != nil {
		return nil, err
	}

	if _, err = pc.AddTransceiverFromTrack(s.videoTrack,
		webrtc.RtpTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		},
	); err != nil {
		return nil, err
	}

	// Create a data channels
	s.audioDataChannel, err = pc.CreateDataChannel("audio", nil)
	if err != nil {
		return nil, err
	}

	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		switch d.Label() {
		case "touch":
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				s.sendTouch(lnk, msg.Data)
			})
		case "start":
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				if err := s.startCarPlay(lnk, msg.Data); err != nil {
					s.Error("start car play", "error", err.Error())
				}
			})
		}
	})

	// Set the remote SessionDescription
	if err := pc.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	// Create an answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	if err = pc.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	return &answer, nil
}

func (s *Server) startCarPlay(lnk *link.Link, data []byte) error {

	if err := json.Unmarshal(data, s.size); err != nil {
		return err
	}

	if err := lnk.SetScreenSize(*s.size); err != nil {
		return err
	}
	go lnk.Communicate(func(data interface{}) {
		switch data := data.(type) {
		case *protocol.VideoData:
			duration := time.Duration((float32(1) / float32(s.fps)) * float32(time.Second))

			s.videoTrack.WriteSample(media.Sample{Data: data.Data, Duration: duration})
		case *protocol.AudioData:
			if len(data.Data) == 0 {
				s.Debug("[onData]", "data", data)
			} else {
				var buf bytes.Buffer
				fr := protocol.AudioDecodeTypes[data.DecodeType].Frequency
				ch := protocol.AudioDecodeTypes[data.DecodeType].Channel
				binary.Write(&buf, binary.LittleEndian, fr)
				binary.Write(&buf, binary.LittleEndian, ch)
				s.audioDataChannel.Send(append(buf.Bytes(), data.Data...))
			}
		default:
			s.Debug("[onData]", "data", data)
		}
	})

	return nil
}

func (s *Server) sendTouch(lnk *link.Link, data []byte) {
	var touch link.ScreenTouch
	if err := json.Unmarshal(data, &touch); err != nil {
		s.Error("unmarshal touch", "error", err.Error())
		return
	}

	lnk.Send(&protocol.Touch{X: uint32(touch.X * 10000 / float32(s.size.Width)), Y: uint32(touch.Y * 10000 / float32(s.size.Height)), Action: protocol.TouchAction(touch.Action)})
}
