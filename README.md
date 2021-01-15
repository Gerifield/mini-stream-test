I was experimenting with some minimal RTMP/stream processing in Go.

I wanted to have access to the frames coming from the stream and tried to compare some of the available libs.

"solutions":
- server is use the `github.com/torresjeff/rtmp` lib itself (it limits the "app" where you should stream right now)
- server2 use parts of the same lib, but most of the parts for one kind of stream has been moved/hardcoded in my code, so it's a bit more flexible (but hard to read because it was only an experiment)


Server start:
```
$ go run cmd/server2/server2.go
```

RTMP streaming:
```
$ ffmpeg -re -i short.mp4 -vcodec libx264 -preset:v ultrafast -video_size 640x480 -acodec aac -f flv rtmp://localhost:8888/something
```

After connection you should see stuff like:
```
$ go run cmd/server2/server2.go                                                                                                                                                         130 ↵
Handshake done
Chunk header size 12
Chunk data size 100
Pl slice: 100
Type ID: 20
tID 1
commandObject map[app:something tcUrl:rtmp://localhost:8888/something type:nonprivate]
CONNECT

Chunk header size 8
Chunk data size 29
Pl slice: 29
Type ID: 20
tID 2
commandObject map[]
releaseStream

Chunk header size 8
Chunk data size 25
Pl slice: 25
Type ID: 20
tID 3
commandObject map[]
FCPublish

Chunk header size 1
Chunk data size 25
Pl slice: 25
Type ID: 20
tID 4
commandObject map[]
CREATE STREAM

Chunk header size 12
Chunk data size 30
Pl slice: 30
Type ID: 20
tID 5
commandObject map[]
PUBLISH  live

Chunk header size 12
Chunk data size 391
Pl slice: 388
Type ID: 18

Chunk header size 12
Chunk data size 44
Pl slice: 44
Type ID: 9
Frame Type 1 Codec 7 Frame size 44 ts 0

Chunk header size 12
Chunk data size 7
Pl slice: 7
Type ID: 8
Format 10 Sample rate 3 Sample size 1 Channels 1

Chunk header size 12
Chunk data size 271
Pl slice: 269
Type ID: 8
Format 10 Sample rate 3 Sample size 1 Channels 1

Chunk header size 8
Chunk data size 177439
Pl slice: 176064
Type ID: 9
Frame Type 1 Codec 7 Frame size 176064 ts 12

Chunk header size 8
Chunk data size 315
Pl slice: 313
Type ID: 8
Format 10 Sample rate 3 Sample size 1 Channels 1
```