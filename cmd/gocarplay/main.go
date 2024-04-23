package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"

	"github.com/mzyy94/gocarplay/link"
	"github.com/mzyy94/gocarplay/protocol"
)

//go:embed *.js
//go:embed *.html
var folder embed.FS

var (
	videoTrack       *webrtc.TrackLocalStaticSample
	audioDataChannel *webrtc.DataChannel
	size             link.ScreenSize
	fps              int32 = 25
)

func setupWebRTC(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	fmt.Println("setupwebrtc")
	// todo make this listen from kill or term signal
	in, out, err := link.Connect(context.Background())
	if err != nil {
		return nil, err
	}
	testFile, err := os.Create("./test_file.txt")
	if err != nil {
		return nil, err
	}
	in2 := io.MultiReader(in, testFile)

	lnk, err := link.New(
		link.WithContext(context.Background()),
		link.WithDPI(160),
		link.WithFPS(fps),
		link.WithReader(in2),
		link.WithWriter(out),
		// link.WithScreenSize(size),
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
		log.Printf("State of %s: %s \n", stats.ID, connectionState.String())
	})

	// Create a video track
	videoCodec := webrtc.RTPCodecCapability{
		MimeType:     webrtc.MimeTypeH264,
		ClockRate:    90000,
		Channels:     0,
		SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
		RTCPFeedback: nil,
	}
	if videoTrack, err = webrtc.NewTrackLocalStaticSample(videoCodec, "video", "video"); err != nil {
		return nil, err
	}

	if _, err = pc.AddTransceiverFromTrack(videoTrack,
		webrtc.RtpTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		},
	); err != nil {
		return nil, err
	}

	// Create a data channels
	audioDataChannel, err = pc.CreateDataChannel("audio", nil)
	if err != nil {
		return nil, err
	}

	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Println("d.Label()", d.Label())
		switch d.Label() {
		case "touch":
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				sendTouch(lnk, msg.Data)
			})
		case "start":
			d.OnMessage(func(msg webrtc.DataChannelMessage) {
				if err := startCarPlay(lnk, msg.Data); err != nil {
					log.Fatalf("start car play: %v", err)
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

func webRTCOfferHandler(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "{\"error\": \"%s\"}", err.Error())
		return
	}

	answer, err := setupWebRTC(offer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{\"error\": \"%s\"}", err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&answer)
}

func sendTouch(lnk *link.Link, data []byte) {
	var touch link.ScreenTouch
	if err := json.Unmarshal(data, &touch); err != nil {
		return
	}

	lnk.Send(&protocol.Touch{X: uint32(touch.X * 10000 / float32(size.Width)), Y: uint32(touch.Y * 10000 / float32(size.Height)), Action: protocol.TouchAction(touch.Action)})
}

func startCarPlay(lnk *link.Link, data []byte) error {
	if err := json.Unmarshal(data, &size); err != nil {
		return err
	}

	fmt.Println("start car play")
	if err := lnk.SetScreenSize(size); err != nil {
		return err
	}
	frameCount := 0
	start := time.Now()
	go lnk.Communicate(func(data interface{}) {
		switch data := data.(type) {
		case *protocol.VideoData:
			duration := time.Duration((float32(1) / float32(fps)) * float32(time.Second))
			frameCount++
			secs := time.Since(start).Seconds()
			fmt.Println("frames", float64(frameCount)/secs, "frameCount", frameCount, "secs", secs)

			videoTrack.WriteSample(media.Sample{Data: data.Data, Duration: duration})
		case *protocol.AudioData:
			if len(data.Data) == 0 {
				log.Printf("[onData] %#v", data)
			} else {
				var buf bytes.Buffer
				fr := protocol.AudioDecodeTypes[data.DecodeType].Frequency
				ch := protocol.AudioDecodeTypes[data.DecodeType].Channel
				binary.Write(&buf, binary.LittleEndian, fr)
				binary.Write(&buf, binary.LittleEndian, ch)
				audioDataChannel.Send(append(buf.Bytes(), data.Data...))
			}
		default:
			log.Printf("[onData] %T TYPE:: %#v", data, data)
		}
	})

	return nil
}

func main() {
	log.Println("http://localhost:8001")
	http.HandleFunc("/connect", webRTCOfferHandler)
	http.Handle("/", http.FileServer(http.FS(folder)))
	log.Fatal(http.ListenAndServe(":8001", nil))
}
