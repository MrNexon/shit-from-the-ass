package main

import (
	"fmt"
	"github.com/aquilax/go-perlin"
	"github.com/eripe970/go-dsp-utils"
	"github.com/gordonklaus/portaudio"
	"github.com/lucasb-eyer/go-colorful"
	"math"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"
)

type AvgSignal struct {
	Value float64
	Max   float64
	Buff  []float64
	index int
}

func (s *AvgSignal) addValue(val float64) {
	s.Buff[s.index] = val

	s.index += 1
	if s.index >= len(s.Buff) {
		s.index = 0
	}

	s.Value = 0
	s.Max = s.Buff[0]
	for i := 0; i < len(s.Buff); i++ {
		s.Value += s.Buff[i]
		if s.Buff[i] > s.Max {
			s.Max = s.Buff[i]
		}
	}
	s.Value /= float64(len(s.Buff))
}

func main() {
	s, err := net.ResolveUDPAddr("udp4", "led.local:9000")
	c, err := net.DialUDP("udp4", nil, s)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("The UDP server is %s\n", c.RemoteAddr().String())
	defer c.Close()

	fmt.Println("Recording.  Press Ctrl-C to stop.")
	portaudio.Initialize()
	defer portaudio.Terminate()
	//in := make([]int32, 64)
	dev, _ := portaudio.Devices()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)

	var targetDevice *portaudio.DeviceInfo
	for _, device := range dev {
		if strings.Index(device.Name, "Soundflower") > -1 {
			targetDevice = device
			fmt.Println(device.Name)
			break
		}

	}

	in := make([]int32, 128)

	//a := 1.
	//add := 0.

	signalAvg := AvgSignal{
		Buff: make([]float64, 200),
	}

	value := AvgSignal{
		Buff: make([]float64, 15),
	}

	windowSize := 50
	maxValue := AvgSignal{
		Buff: make([]float64, windowSize),
	}

	diffAvg := AvgSignal{
		Buff: make([]float64, 300),
	}

	bitAvg := AvgSignal{
		Buff: make([]float64, 3),
	}
	/*minValue := AvgSignal{
		Buff: make([]float64, windowSize),
	}*/

	a := 0.
	bit := 0.
	saturationSub := 0.
	saturationSubActive := false

	stream, err := portaudio.OpenStream(portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   targetDevice,
			Channels: 1,
			Latency:  5,
		},
		SampleRate:      44200,
		FramesPerBuffer: len(in),
	}, in)

	chk(err)
	defer stream.Close()

	chk(stream.Start())

	for {
		chk(stream.Read())

		inFloat := make([]float64, len(in))
		for i := 0; i < len(in); i++ {
			inFloat[i] = float64(in[i])
		}

		audioSignal := dsp.Signal{
			SampleRate: 44100,
			Signal:     inFloat,
		}

		sigV := (audioSignal.Max() - audioSignal.Min()) / 2147483647
		signalAvg.addValue(sigV)

		lowAudio, _ := audioSignal.LowPassFilter(400)

		lowV := (lowAudio.Max() - lowAudio.Min()) / 2147483647
		value.addValue(lowV)

		maxValue.addValue(value.Value)
		//minValue.addValue(minMinV)

		/*lowVolume := (lowMaxV - lowMinV) / 2147483647
		lowAvg.addValue(lowVolume)*/

		data := make([]byte, 901)

		diff := math.Max(value.Value-maxValue.Value, 0)
		diffAvg.addValue(diff)

		valPix := int(convert(value.Value, 0, 300))
		maxPix := int(convert(maxValue.Value, 0, 300))
		minPix := int(convert(diff, 0, 300))
		diffAvgPix := int(convert(diffAvg.Value*2, 0, 300))

		if diff >= (diffAvg.Value*2) && bit == 0 {
			bit = math.Max(diff-(diffAvg.Value*2), 0)
			go func() {
				<-time.After(50 * time.Millisecond)
				bit = 0
			}()
		}

		bitAvg.addValue(bit)

		a += 0.003 + (signalAvg.Value * 0.01)
		p := perlin.NewPerlin(1, 1, 1, 10)

		data[0] = 150
		if bitAvg.Value > 0.04 && signalAvg.Value > 0.4 && !saturationSubActive {
			saturationSub = 0.3
			saturationSubActive = true
			go func() {
				<-time.After(50 * time.Millisecond)
				speed := 100
				step := saturationSub / float64(speed)
				for i := 0; i < speed; i++ {
					saturationSub -= step
					<-time.After(1 * time.Millisecond)
				}
				saturationSubActive = false
			}()
		}

		for x := 0; x < 300; x++ {

			noiseX := float64(x) / 10
			noiseY := a
			noiseV := p.Noise2D(noiseX, noiseY) * ((signalAvg.Value * 1.8) + 0.3)

			hueBase := 240.
			hueRange := 60.
			signalAdd := 0. //( * 1.8) * 60

			col := colorful.Hsv(hueBase+(noiseV*hueRange)+signalAdd, 1-saturationSub, math.Min(signalAvg.Value+0.1, 1))
			r, g, b := col.RGB255()
			data[(x*3)+1] = r
			data[(x*3)+2] = g
			data[(x*3)+3] = b

			if valPix == x {
				data[(x*3)+1] = 255
			}
			if maxPix == x || diffAvgPix == x {
				data[(x*3)+2] = 255
			}
			if minPix == x {
				data[(x*3)+3] = 255
			}
		}

		_, err = c.Write(data)
		if err != nil {
			panic(err)
		}

		select {
		case <-sig:
			return
		default:
		}

		_ = valPix
		_ = maxPix
		_ = minPix
		_ = diffAvgPix
	}
	chk(stream.Stop())
}

func chk(err error) {
	if err != nil {
		return
	}
}

func min(array []float64) float64 {
	result := array[0]
	for _, el := range array {
		if el < result {
			result = el
		}
	}

	return result
}

func max(array []float64) float64 {
	result := array[0]
	for _, el := range array {
		if el > result {
			result = el
		}
	}

	return result
}

func avg(arr *[]float64, index *int, smooth int, value float64) float64 {
	(*arr)[*index] = value
	*index += 1
	if *index >= smooth {
		*index = 0
	}

	var amplitudeValue float64 = 0
	for i := 0; i < smooth; i++ {
		amplitudeValue += (*arr)[i]
	}

	return amplitudeValue / float64(smooth)
}

func convert(val float64, min float64, max float64) float64 {
	return math.Min(math.Max(min+(max*val), min), max)
}
