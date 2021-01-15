package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/torresjeff/rtmp"
	"github.com/torresjeff/rtmp/amf/amf0"
	"github.com/torresjeff/rtmp/audio"
	"github.com/torresjeff/rtmp/video"
)

// Thanks to github.com/torresjeff/rtmp

var ErrUnsupportedRTMPVersion error = errors.New("the version of RTMP is not supported")
var ErrWrongC2Message error = errors.New("server handshake: s1 and c2 handshake messages do not match")

//var ErrWrongS2Message error = errors.New("client handshake: c1 and s2 handshake messages do not match")

const RtmpVersion3 = 3

func main() {

	listener, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatalln(err)
	}

	// Loop infinitely, accepting any incoming connection. Every new connection will create a new session.
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}

		go func() {
			connReader := bufio.NewReaderSize(conn, 1024*64)
			connWriter := bufio.NewWriterSize(conn, 1024*64)
			err := Handshake(connReader, connWriter)
			if err == io.EOF {
				_ = conn.Close()
				return
			} else if err != nil {
				_ = conn.Close()
				fmt.Println("handshake error:", err)
				return
			}

			fmt.Println("Handshake done")

			//// TODO: read chunks?
			//buff := make([]byte, 128)
			//for {
			//	n, err := conn.Read(buff)
			//	if err != nil {
			//		fmt.Println(string(buff[:n]))
			//	}
			//}

			ch := rtmp.NewChunkHandler(connReader, connWriter)
			for {
				header, hsize, err := ch.ReadChunkHeader()
				if err != nil {
					fmt.Println("header read fail", err)
					return
				}
				fmt.Println("Chunk header size", hsize)

				pl, plsize, err := ch.ReadChunkData(header)
				if err != nil {
					fmt.Println("data read error", err)
					return
				}

				fmt.Println("Chunk data size", plsize)
				fmt.Println("Pl slice:", len(pl))

				// Interpret and ack
				fmt.Println("Type ID:", header.MessageHeader.MessageTypeID)
				//CommandMessageAMF0 -> 20
				switch header.MessageHeader.MessageTypeID {
				case 20: // CommandMessageAMF0
					commandName, err := amf0.Decode(pl) // Decode the command name (always the first string in the payload)
					if err != nil {
						fmt.Println("amf0 decode error", err)
					}
					handleCommandAmf0(connWriter, header.BasicHeader.ChunkStreamID, header.MessageHeader.MessageStreamID, commandName.(string), pl[amf0.Size(commandName.(string)):])

				case 8: // AudioMessage
					handleAudioMessage(header.BasicHeader.ChunkStreamID, header.MessageHeader.MessageStreamID, pl, header.ElapsedTime)
				case 9: // VideoMessage
					handleVideoMessage(header.BasicHeader.ChunkStreamID, header.MessageHeader.MessageStreamID, pl, header.ElapsedTime)

					// Cheat sheet:
					//	CommandMessageAMF0 = 20
					//	CommandMessageAMF3 = 17
					//
					//	DataMessageAMF0 = 18
					//	DataMessageAMF3 = 15
					//
					//	SharedObjectMessageAMF0 = 19
					//	SharedObjectMessageAMF3 = 16
					//
					//	AudioMessage = 8
					//	VideoMessage = 9
					//	AggregateMessage = 22

				}
				fmt.Println() // empty line
			}
		}()
	}
}

func handleAudioMessage(chunkStreamID uint32, messageStreamID uint32, payload []byte, timestamp uint32) {
	// Header contains sound format, rate, size, type
	audioHeader := payload[0]
	format := audio.Format((audioHeader >> 4) & 0x0F)
	sampleRate := audio.SampleRate((audioHeader >> 2) & 0x03)
	sampleSize := audio.SampleSize((audioHeader >> 1) & 1)
	channels := audio.Channel((audioHeader) & 1)

	// TODO: Do I need this?
	// Cache aac sequence header to send to play back clients when they connect
	//if format == audio.AAC && audio.AACPacketType(payload[1]) == audio.AACSequenceHeader {
	//	session.broadcaster.SetAacSequenceHeaderForPublisher(session.streamKey, payload)
	//}

	fmt.Println("Format", format, "Sample rate", sampleRate, "Sample size", sampleSize, "Channels", channels)
}

func handleVideoMessage(csID uint32, messageStreamID uint32, payload []byte, timestamp uint32) {
	// Header contains frame type (key frame, i-frame, etc.) and format/codec (H264, etc.)
	videoHeader := payload[0]
	frameType := video.FrameType((videoHeader >> 4) & 0x0F)
	codec := video.Codec(videoHeader & 0x0F)

	// TODO: Do I need this?
	// cache avc sequence header to send to playback clients when they connect
	//if codec == video.H264 && video.AVCPacketType(payload[1]) == video.AVCSequenceHeader {
	//	session.broadcaster.SetAvcSequenceHeaderForPublisher(session.streamKey, payload)
	//}

	fmt.Println("Frame Type", frameType, "Codec", codec, "Frame size", len(payload), "ts", timestamp)
}

func handleCommandAmf0(connWriter *bufio.Writer, csID uint32, streamID uint32, commandName string, payload []byte) {
	// Every command has a transaction ID and a command object (which can be null)
	tId, _ := amf0.Decode(payload)
	byteLength := amf0.Size(tId)
	transactionID := tId.(float64)
	// Update our payload to read the next property (commandObject)
	payload = payload[byteLength:]
	cmdObject, _ := amf0.Decode(payload)
	var commandObject map[string]interface{}
	switch cmdObject.(type) {
	case nil:
		commandObject = nil
	case map[string]interface{}:
		commandObject = cmdObject.(map[string]interface{})
	case amf0.ECMAArray:
		commandObject = cmdObject.(amf0.ECMAArray)
	}
	// Update our payload to read the next property
	byteLength = amf0.Size(cmdObject)
	payload = payload[byteLength:]

	fmt.Println("tID", transactionID)
	fmt.Println("commandObject", commandObject)

	switch commandName {
	case "connect":
		fmt.Println("CONNECT")
		// Check stream key and stuff here
		// STEP 1

		// Initiate connect sequence
		// As per the specification, after the connect command, the server sends the protocol message Window Acknowledgment Size
		//session.messageManager.sendWindowAckSize(config.DefaultClientWindowSize)
		connWriter.Write(generateWindowAckSizeMessage(2500000))
		//connWriter.Flush()

		// After sending the window ack size message, the server sends the set peer bandwidth message
		//session.messageManager.sendSetPeerBandWidth(config.DefaultClientWindowSize, LimitDynamic)
		connWriter.Write(generateSetPeerBandwidthMessage(2500000, 2))
		//connWriter.Flush()

		// Send the User Control Message to begin stream with stream ID = DefaultPublishStream (which is 0)
		// Subsequent messages sent by the client will have stream ID = DefaultPublishStream, until another sendBeginStream message is sent
		//session.messageManager.sendBeginStream(config.DefaultPublishStream)
		connWriter.Write(generateStreamBeginMessage(0))
		//connWriter.Flush()

		// Send Set Chunk Size message
		//session.messageManager.sendSetChunkSize(config.DefaultChunkSize)
		connWriter.Write(generateSetChunkSizeMessage(4096))
		//connWriter.Flush()

		// Send Connect Success response
		//session.messageManager.sendConnectSuccess(csID)
		connWriter.Write(generateConnectResponseSuccess(csID))
		connWriter.Flush()

	case "releaseStream":
		streamKey, _ := amf0.Decode(payload)
		fmt.Println("releaseStream", streamKey.(string))
	case "FCPublish":
		streamKey, _ := amf0.Decode(payload)
		fmt.Println("FCPublish", streamKey.(string))
	case "createStream":
		fmt.Println("CREATE STREAM")
		// STEP 2

		connWriter.Write(generateCreateStreamResponse(csID, transactionID, commandObject))
		connWriter.Write(generateStreamBeginMessage(1))
		connWriter.Flush()

	case "publish":
		// name with which the stream is published (basically the streamKey)
		streamKey, _ := amf0.Decode(payload)
		byteLength = amf0.Size(streamKey)
		payload = payload[byteLength:]
		// Publishing type: "live", "record", or "append"
		// - record: The stream is published and the data is recorded to a new file. The file is stored on the server
		// in a subdirectory within the directory that contains the server application. If the file already exists, it is overwritten.
		// - append: The stream is published and the data is appended to a file. If no file is found, it is created.
		// - live: Live data is published without recording it in a file.
		publishingType, _ := amf0.Decode(payload)

		fmt.Println("PUBLISH", streamKey.(string), publishingType.(string))

		// STEP 3

		sendStatusMessage(connWriter, "status", "NetStream.Publish.Start", "Publishing live_user_<x>")

	case "play":
		streamKey, _ := amf0.Decode(payload)
		byteLength = amf0.Size(streamKey)
		payload = payload[byteLength:]

		// Start time in seconds
		startTime, _ := amf0.Decode(payload)
		byteLength = amf0.Size(startTime)
		payload = payload[byteLength:]

		// the spec specifies that, the next values should be duration (number), and reset (bool), but VLC doesn't send them
		fmt.Println("PLAY", streamKey.(string), startTime.(float64))
	case "FCUnpublish":
		streamKey, _ := amf0.Decode(payload)
		fmt.Println("FCUnpublish", streamKey.(string))
	case "closeStream":
		fmt.Println("closeStream")
	case "deleteStream":
		streamID, _ := amf0.Decode(payload)
		fmt.Println("deleteStream", streamID.(float64))
	case "_result":
		info, _ := amf0.Decode(payload)
		fmt.Println("RESULT", info.(map[string]interface{}))
	case "onStatus":
		info, _ := amf0.Decode(payload)
		fmt.Println("STATUS", info.(map[string]interface{}))
	default:
		fmt.Println("message manager: received command " + commandName + ", but couldn't handle it because no implementation is defined")
	}
}

func Handshake(reader *bufio.Reader, writer *bufio.Writer) error {
	c1, err := readC0C1(reader)
	if err != nil {
		return err
	}
	s1, err := sendS0S1S2(writer, c1)
	if err != nil {
		return err
	}
	c2, err := readC2(reader)
	if err != nil {
		return err
	}

	if bytes.Compare(s1, c2) != 0 {
		return ErrWrongC2Message
	}

	return nil
}

func readC0C1(reader *bufio.Reader) ([]byte, error) {
	var c0c1 [1537]byte

	if _, err := io.ReadFull(reader, c0c1[:]); err != nil {
		return nil, err
	}

	if c0c1[0] != RtmpVersion3 {
		return nil, ErrUnsupportedRTMPVersion
	}

	// Returns c1 message
	return c0c1[1:], nil
}

func sendS0S1S2(writer *bufio.Writer, c1 []byte) ([]byte, error) {
	var s0s1s2 [1 + 2*1536]byte
	var err error
	// s0 message is stored in byte 0
	s0s1s2[0] = RtmpVersion3
	// s1 message is stored in bytes 1-1536
	if err = generateRandomData(s0s1s2[1:1537]); err != nil {
		return nil, err
	}
	// s2 message is stored in bytes 1537-3073
	copy(s0s1s2[1537:], c1)

	err = send(writer, s0s1s2[:])
	if err != nil {
		return nil, err
	}
	return s0s1s2[1:1537], nil
}

// Returns the C2 message
func readC2(reader *bufio.Reader) ([]byte, error) {
	var c2 [1536]byte
	if _, err := io.ReadFull(reader, c2[:]); err != nil {
		return nil, err
	}
	return c2[:], nil
}

func sendStatusMessage(connWriter *bufio.Writer, level string, code string, description string, optionalDetails ...string) {
	infoObject := map[string]interface{}{
		"level":       level,
		"code":        code,
		"description": description,
	}
	if len(optionalDetails) > 0 && optionalDetails[0] != "" {
		infoObject["details"] = optionalDetails[0]
	}

	message := generateStatusMessage(0, 0, infoObject)
	_, err := connWriter.Write(message)
	if err != nil {
		fmt.Println("error sending status message:", err)
	}
	err = connWriter.Flush()
	if err != nil {
		fmt.Println("error sending status message flush:", err)
	}
}

// Generates an S1 message (random data)
func generateRandomData(s1 []byte) error {
	// the s1 byte array is zero-initialized, since we didn't modify it, we're sending our time as 0
	err := GenerateRandomDataFromBuffer(s1[8:])
	if err != nil {
		return err
	}
	return nil
}

func GenerateRandomDataFromBuffer(b []byte) error {
	_, err := rand.Read(b)
	if err != nil {
		return err
	}
	return nil
}

func send(writer *bufio.Writer, bytes []byte) error {
	if _, err := writer.Write(bytes); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return nil
}

func generateWindowAckSizeMessage(size uint32) []byte {
	windowAckSizeMessage := make([]byte, 16)
	//---- HEADER ----//
	// fmt = 0 and csid = 2 encoded in 1 byte
	windowAckSizeMessage[0] = 2
	// timestamp (3 bytes) is set to 0 so bytes 1-3 are unmodified (they're already zero-initialized)
	//windowAckSizeMessage[1] = 0
	//windowAckSizeMessage[2] = 0
	//windowAckSizeMessage[3] = 0

	// the next 3 bytes (4-6) indicate the size of the body which is 4 bytes. So set it to 4 (the first 2 bytes 4-5 are unused because the number 4 only requires 1 byte to store)
	windowAckSizeMessage[6] = 4

	// Set the type of the message. In our case this is a Window Acknowledgement Size message (5)
	//windowAckSizeMessage[7] = WindowAckSize
	windowAckSizeMessage[7] = 5

	// The next 4 bytes indicate the Message Stream ID. Protocol Control Messages, such as Window Acknowledgement Size, always use the message stream ID 0. So leave them at 0
	// (they're already zero initialized)
	// NetConnection is the default communication channel, which has a stream ID 0. Protocol and a few command messages, including createStream, use the default communication channel.
	//windowAckSizeMessage[8] = 0
	//windowAckSizeMessage[9] = 0
	//windowAckSizeMessage[10] = 0
	//windowAckSizeMessage[11] = 0

	//---- BODY ----//
	// Finally, store the actual window size message the client should use in the last 4 bytes
	binary.BigEndian.PutUint32(windowAckSizeMessage[12:], size)

	return windowAckSizeMessage
}

func generateSetPeerBandwidthMessage(size uint32, limitType uint8) []byte {
	setPeerBandwidthMessage := make([]byte, 17)
	//---- HEADER ----//
	// fmt = 0 and csid = 2 encoded in 1 byte.
	// Chunk Stream ID with value 2 is reserved for low-level protocol control messages and commands.
	setPeerBandwidthMessage[0] = 2
	// timestamp (3 bytes) is set to 0 so bytes 1-3 are unmodified (they're already zero-initialized)
	//setPeerBandwidthMessage[1] = 0
	//setPeerBandwidthMessage[2] = 0
	//setPeerBandwidthMessage[3] = 0

	// the next 3 bytes (4-6) indicate the size of the body which is 5 bytes (4 bytes for the window ack size, 1 byte for the limit type)
	// So set it to 5 (the first 2 bytes 4-5 are unused because the number 5 only requires 1 byte to store)
	setPeerBandwidthMessage[6] = 5

	// Set the type of the message. In our case this is a Set Peer Bandwidth message (5)
	//setPeerBandwidthMessage[7] = SetPeerBandwidth
	setPeerBandwidthMessage[7] = 6

	// The next 4 bytes indicate the Message Stream ID. Protocol Control Messages, such as Window Acknowledgement Size, always use the message stream ID 0. So leave them at 0
	// (they're already zero initialized)
	// NetConnection is the default communication channel, which has a stream ID 0. Protocol and a few command messages, including createStream, use the default communication channel.
	//setPeerBandwidthMessage[8] = 0
	//setPeerBandwidthMessage[9] = 0
	//setPeerBandwidthMessage[10] = 0
	//setPeerBandwidthMessage[11] = 0

	//---- BODY ----//
	// Store the actual peer bandwidth the client should use in the next 4 bytes
	binary.BigEndian.PutUint32(setPeerBandwidthMessage[12:], size)

	// Finally, set the limit type (hard = 0, soft = 1, dynamic = 2). The spec defines each one of these as follows:
	// 0 - Hard: The peer SHOULD limit its output bandwidth to the indicated window size.
	// 1 - Soft: The peer SHOULD limit its output bandwidth to the the window indicated in this message or the limit already in effect, whichever is smaller.
	// 2 - Dynamic: If the previous Limit Type was Hard, treat this message as though it was marked Hard, otherwise ignore this message.
	setPeerBandwidthMessage[16] = limitType

	return setPeerBandwidthMessage
}

func generateStreamBeginMessage(streamId uint32) []byte {
	streamBeginMessage := make([]byte, 18)
	//---- HEADER ----//
	// fmt = 0 and csid = 2 encoded in 1 byte.
	// Chunk Stream ID with value 2 is reserved for low-level protocol control messages and commands.
	streamBeginMessage[0] = 2
	// timestamp (3 bytes) is set to 0 so bytes 1-3 are unmodified (they're already zero-initialized)
	//streamBeginMessage[1] = 0
	//streamBeginMessage[2] = 0
	//streamBeginMessage[3] = 0

	// the next 3 bytes (4-6) indicate the size of the body which is 6 bytes (2 bytes for the event type, 4 bytes for the event data)
	// So set it to 6 (the first 2 bytes 4-5 are unused because the number 6 only requires 1 byte to store)
	streamBeginMessage[6] = 6

	// Set the type of the message. In our case this is a User Control Message (4)
	//streamBeginMessage[7] = UserControlMessage
	streamBeginMessage[7] = 4

	// The next 4 bytes indicate the Message Stream ID. Protocol Control Messages, such as Window Acknowledgement Size, always use the message stream ID 0. So leave them at 0.
	// (they're already zero initialized)
	// NetConnection is the default communication channel, which has a stream ID 0. Protocol and a few command messages, including createStream, use the default communication channel.
	//streamBeginMessage[8] = 0
	//streamBeginMessage[9] = 0
	//streamBeginMessage[10] = 0
	//streamBeginMessage[11] = 0

	//---- BODY ----//
	// The next two bytes specify the the event type. In our case, since this is a Stream Begin message, it has event type = 0. Leave the next two bytes at 0.
	//streamBeginMessage[12] = 0
	//streamBeginMessage[13] = 0

	// The next 4 bytes specify the event the stream ID that became functional
	binary.BigEndian.PutUint32(streamBeginMessage[14:], streamId)

	return streamBeginMessage
}

func generateSetChunkSizeMessage(chunkSize uint32) []byte {
	setChunkSizeMessage := make([]byte, 16)
	//---- HEADER ----//
	// fmt = 0 and csid = 2 encoded in 1 byte
	// Chunk Stream ID with value 2 is reserved for low-level protocol control messages and commands.
	setChunkSizeMessage[0] = 2
	// timestamp (3 bytes) is set to 0 so bytes 1-3 are unmodified (they're already zero-initialized)
	//setChunkSizeMessage[1] = 0
	//setChunkSizeMessage[2] = 0
	//setChunkSizeMessage[3] = 0

	// the next 3 bytes (4-6) indicate the inChunkSize of the body which is 4 bytes. So set it to 4 (the first 2 bytes 4-5 are unused because the number 4 only requires 1 byte to store)
	setChunkSizeMessage[6] = 4

	// Set the type of the message. In our case this is a Window Acknowledgement Size message (5)
	//setChunkSizeMessage[7] = SetChunkSize
	setChunkSizeMessage[7] = 1

	// The next 4 bytes indicate the Message Stream ID. Protocol Control Messages, such as Window Acknowledgement Size, always use the message stream ID 0. So leave them at 0
	// (they're already zero initialized)
	// NetConnection is the default communication channel, which has a stream ID 0. Protocol and a few command messages, including createStream, use the default communication channel.
	//setChunkSizeMessage[8] = 0
	//setChunkSizeMessage[9] = 0
	//setChunkSizeMessage[10] = 0
	//setChunkSizeMessage[11] = 0

	//---- BODY ----//
	// Finally, store the actual chunk size client should use in the last 4 bytes
	binary.BigEndian.PutUint32(setChunkSizeMessage[12:], chunkSize)

	return setChunkSizeMessage
}

func generateConnectResponseSuccess(csID uint32) []byte {
	// Body of our message
	commandName, _ := amf0.Encode("_result")
	// Transaction ID is 1 for connection responses
	transactionId, _ := amf0.Encode(1)
	properties, _ := amf0.Encode(map[string]interface{}{
		"fmsVer":       "FMS/3,5,7,7009",
		"capabilities": 31,
		"mode":         1,
	})
	information, _ := amf0.Encode(map[string]interface{}{
		"code":        "NetConnection.Connect.Success",
		"level":       "status",
		"description": "Connection accepted.",
		"data": map[string]interface{}{
			"string": "3,5,7,7009",
		},
		"objectEncoding": 0, // AMFVersion0
	})

	// Calculate the body length
	bodyLength := len(commandName) + len(transactionId) + len(properties) + len(information)

	// 12 bytes for the header
	connectResponseSuccessMessage := make([]byte, 12, 300)
	//---- HEADER ----//
	// fmt = 0 and csid = 3 encoded in 1 byte
	// why does Twitch send csId = 3? is it because it is replying to the connect() request which sent csID = 3?
	connectResponseSuccessMessage[0] = byte(csID)

	// timestamp (3 bytes) is set to 0 so bytes 1-3 are unmodified (they're already zero-initialized)
	//connectResponseSuccessMessage[1] = 0
	//connectResponseSuccessMessage[2] = 0
	//connectResponseSuccessMessage[3] = 0

	// Bytes 4-6 specify the body size.
	connectResponseSuccessMessage[4] = byte((bodyLength >> 16) & 0xFF)
	connectResponseSuccessMessage[5] = byte((bodyLength >> 8) & 0xFF)
	connectResponseSuccessMessage[6] = byte(bodyLength)

	// Set type to AMF0 command (20)
	//connectResponseSuccessMessage[7] = CommandMessageAMF0
	connectResponseSuccessMessage[7] = 20

	// Next 4 bytes specify the stream ID, leave it at 0
	// NetConnection is the default communication channel, which has a stream ID 0. Protocol and a few command messages, including createStream, use the default communication channel.
	//connectResponseSuccessMessage[8] = 0
	//connectResponseSuccessMessage[9] = 0
	//connectResponseSuccessMessage[10] = 0
	//connectResponseSuccessMessage[11] = 0

	//---- BODY ----//
	// Set the body
	connectResponseSuccessMessage = append(connectResponseSuccessMessage, commandName...)
	connectResponseSuccessMessage = append(connectResponseSuccessMessage, transactionId...)
	connectResponseSuccessMessage = append(connectResponseSuccessMessage, properties...)
	connectResponseSuccessMessage = append(connectResponseSuccessMessage, information...)

	return connectResponseSuccessMessage
}

func generateCreateStreamResponse(csID uint32, transactionID float64, commandObject map[string]interface{}) []byte {
	result, _ := amf0.Encode("_result")
	tID, _ := amf0.Encode(transactionID)
	commandObjectResponse, _ := amf0.Encode(nil)
	// ID of the stream that was opened. We could also send an object with additional information if an error occurred, instead of a number.
	// Subsequent chunks will be sent by the client on the stream ID specified here.
	// TODO: is this a fixed value?
	streamID, _ := amf0.Encode(1) // Stream ID
	bodyLength := len(result) + len(tID) + len(commandObjectResponse) + len(streamID)

	createStreamResponseMessage := make([]byte, 12, 50)
	//---- HEADER ----//
	// if csid = 3, does this mean these are not control messages/commands?
	createStreamResponseMessage[0] = byte(csID)

	// Leave timestamp at 0 (bytes 1-3)

	// Set body size (bytes 4-6) to bodyLength
	createStreamResponseMessage[4] = byte((bodyLength >> 16) & 0xFF)
	createStreamResponseMessage[5] = byte((bodyLength >> 8) & 0xFF)
	createStreamResponseMessage[6] = byte(bodyLength)

	// Set type to AMF0 command (20)
	//createStreamResponseMessage[7] = CommandMessageAMF0
	createStreamResponseMessage[7] = 20

	// Leave stream ID at 0 (bytes 8-11)
	// NetConnection is the default communication channel, which has a stream ID 0. Protocol and a few command messages, including createStream, use the default communication channel.

	//---- BODY ----//
	createStreamResponseMessage = append(createStreamResponseMessage, result...)
	createStreamResponseMessage = append(createStreamResponseMessage, tID...)
	createStreamResponseMessage = append(createStreamResponseMessage, commandObjectResponse...)
	createStreamResponseMessage = append(createStreamResponseMessage, streamID...)

	return createStreamResponseMessage
}

func generateStatusMessage(transactionID float64, streamID uint32, infoObject map[string]interface{}) []byte {

	commandName, _ := amf0.Encode("onStatus")
	tID, _ := amf0.Encode(transactionID)
	// Status messages don't have a command object, so encode nil
	commandObject, _ := amf0.Encode(nil)
	info, _ := amf0.Encode(infoObject)
	bodyLength := len(commandName) + len(tID) + len(commandObject) + len(info)

	createStreamResponseMessage := make([]byte, 12, 150)
	//---- HEADER ----//
	// if csid = 3, does this mean these are not control messages/commands?
	createStreamResponseMessage[0] = 3 // Twitch sends 3

	// Leave timestamp at 0 (bytes 1-3)

	// Set body size (bytes 4-6) to bodyLength
	createStreamResponseMessage[4] = byte((bodyLength >> 16) & 0xFF)
	createStreamResponseMessage[5] = byte((bodyLength >> 8) & 0xFF)
	createStreamResponseMessage[6] = byte(bodyLength)

	// Set type to AMF0 command (20)
	//createStreamResponseMessage[7] = CommandMessageAMF0
	createStreamResponseMessage[7] = 20

	// Set stream ID to whatever stream ID the request had (bytes 8-11). Stream ID is stored in LITTLE ENDIAN format.
	binary.LittleEndian.PutUint32(createStreamResponseMessage[8:], streamID)

	//---- BODY ----//
	createStreamResponseMessage = append(createStreamResponseMessage, commandName...)
	createStreamResponseMessage = append(createStreamResponseMessage, tID...)
	createStreamResponseMessage = append(createStreamResponseMessage, commandObject...)
	createStreamResponseMessage = append(createStreamResponseMessage, info...)

	return createStreamResponseMessage
}
