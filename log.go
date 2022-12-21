package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/go-vgo/robotgo"
)

type LogBuf struct {
	serial   int
	id       int
	capacity int
	len      int
	buf      []byte
}

func (lb *LogBuf) Write(str string) bool {
	if lb.len+len(str) > lb.capacity {
		return true
	}

	lb.buf = append(lb.buf, []byte(str)...)
	lb.len += len(str)
	return false
}

func (lb *LogBuf) Reset() *LogBuf {
	lb.len = 0
	return lb
}

const BUF_SIZE = 1024 * 1024

func testLogX() {
	bufCounter := 0
	bufPool := &sync.Pool{
		New: func() interface{} {
			bufCounter++
			return &LogBuf{
				serial:   bufCounter,
				capacity: BUF_SIZE,
				len:      0,
				buf:      make([]byte, 0, BUF_SIZE),
			}
		},
	}

	channel := make(chan *LogBuf, 1024)

	wt := sync.WaitGroup{}

	content := make([]byte, BUF_SIZE/10+1)
	for i := 0; i < BUF_SIZE/10; i++ {
		content[i] = 'a'
	}
	largeStr := string(content)

	// log mem writer
	wt.Add(1)
	go func() {
		var buffer *LogBuf
		id := 1
		for {
			if buffer == nil {
				buffer = bufPool.Get().(*LogBuf)
				fmt.Printf("get buffer #%v\n", buffer.serial)
				buffer.id = id
				id++
			}

			if buffer.Write(largeStr) {
				channel <- buffer
				fmt.Printf("completed buffer #%v size = %v\n", buffer.id, buffer.len)
				buffer = nil
				continue
			} else {
				//fmt.Printf("append %v bytes to buffer#%v\n", len(largeStr), buffer.id)
			}

			time.Sleep(time.Millisecond * 100)
		}
	}()

	// log file writer
	wt.Add(1)
	go func() {
		var buffer *LogBuf

		for {
			buffer = <-channel
			fmt.Printf("received buffer#%v with size = %v\n", buffer.id, buffer.len)
			//time.Sleep(1 * time.Second)
			fmt.Printf("flush buffer #%v to disk size = %v\n", buffer.id, buffer.len)

			bufPool.Put(buffer.Reset())
		}
	}()

	for {
		fmt.Printf("buf total = %d\n", bufCounter)
		time.Sleep(time.Second)
	}

	wt.Wait()
}

func testLog_main() {
	var m sync.Mutex

	c := sync.NewCond(&m)
	go func() {
		for {
			//fmt.Printf("1 sent signal\n\n")
			c.Signal()
			//time.Sleep(time.Second * 5)
		}
	}()

	for i := range []int{1, 2, 3, 4, 5} {
		go func(i int) {
			for {
				m.Lock()
				reCh := make(chan struct{})
				go func() {
					c.Wait()
					reCh <- struct{}{}
				}()
				fmt.Printf("2#%v waking on signal\n", i)
				m.Unlock()
				//fmt.Printf("2#%v waiting signal\n", i)
			}
		}(i)
	}

	time.Sleep(time.Hour)
}

func waitForNewSecond(now time.Time) {
	remain := time.Second - (now.Sub(now.Truncate(time.Second)))
	fmt.Printf("now: %v remain:%v\n", now, remain)
	<-time.After(remain)
}

func testRoundToSecondLogic() {
	var res []time.Time
	for i := 0; i < 3; i++ {
		waitForNewSecond(time.Now())
		res = append(res, time.Now())
		//fmt.Printf("%v\n", time.Now())
		time.Sleep(time.Millisecond * 100)
	}
	for i := range res {
		fmt.Printf("%v\n", res[i])
	}
}

func testReadClosedChannel() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	signal := make(chan int)
	go func() {
		i := 0
		for range ticker.C {
			fmt.Printf("now: %v\n", time.Now())
			time.Sleep(1500 * time.Millisecond)

			i++
			if i > 2 {
				ticker.Stop()
				break
			}
		}
		close(signal)
	}()

	for {
		_, ok := <-signal
		if !ok {
			break
		}
	}

	fmt.Printf("Finished\n")
}

func RunWithTimeout(runner func(), duration time.Duration) error {
	ch := make(chan struct{})

	go func() {
		runner()
		ch <- struct{}{}
	}()

	select {
	case <-ch:
		return nil
	case <-time.After(duration):
		return errors.New("timeout")
	}
}

func testRunWithTimeout() {
	if RunWithTimeout(func() {
		//time.Sleep(time.Second)
		time.Sleep(time.Second + time.Millisecond)
	}, time.Second) != nil {
		fmt.Printf("Timeout\n")
	} else {
		fmt.Printf("execution finished\n")
	}
}

func testContextWithTimeout() {
	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)

	var wg = sync.WaitGroup{}
	wg.Add(1)

	go func() {
		select {
		case _, ok := <-ctx.Done():
			fmt.Printf("ctx canceled, ok = %v\n", ok)
			wg.Done()
		}

	}()

	//cancel() // uncomment this will ignore timeout specified in context.WithTimeout
	wg.Wait()
}

func testContextWithDeadline() {
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(3*time.Second))

	var wg = sync.WaitGroup{}
	wg.Add(1)

	go func() {
		select {
		case _, ok := <-ctx.Done():
			fmt.Printf("ctx canceled, ok = %v\n", ok)
			wg.Done()
		}

	}()

	//cancel() // uncomment this will ignore timeout specified in context.WithTimeout
	wg.Wait()
}

func testContextWithCancel() {
	ctx, cancel := context.WithCancel(context.Background())

	var wg = sync.WaitGroup{}
	wg.Add(1)

	go func() {
		select {
		case _, ok := <-ctx.Done():
			fmt.Printf("ctx canceled, ok = %v\n", ok)
			wg.Done()
		}

	}()

	cancel() // !!! comment this will cause deadlock, main thread and child goroutine will never resume from execution suspension
	wg.Wait()
}

func uncompilableCode() {
	done := make(chan interface{})
	defer close(done)

	//zeros := take(done, 3, repeat(done, 0))
}

func testHttp() {
	hello := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "hello\n")
		fmt.Printf("%v: received request %v\n", time.Now(), req)
	}

	headers := func(w http.ResponseWriter, req *http.Request) {
		for name, headers := range req.Header {
			for _, h := range headers {
				fmt.Fprintf(w, "%v: %v\n", name, h)
				fmt.Printf("%v: received request %v\n", time.Now(), req)
			}
		}
	}

	get := func(w http.ResponseWriter, req *http.Request) {
		k := req.URL.Query().Get("k")
		if client != nil {
			v := client.Get(k).String()
			w.Write([]byte(fmt.Sprintf("k=%v, v=%v", k, v)))
		} else {
			w.Write([]byte(fmt.Sprintf("redis disconnected")))
		}
	}

	ttl := func(w http.ResponseWriter, req *http.Request) {
		k := req.URL.Query().Get("k")
		if client != nil {
			ttl := client.TTL(k).String()
			w.Write([]byte(fmt.Sprintf("k=%v, ttl=%v", k, ttl)))
		} else {
			w.Write([]byte(fmt.Sprintf("redis disconnected")))
		}
	}

	set := func(w http.ResponseWriter, req *http.Request) {
		k := req.URL.Query().Get("k")
		v := req.URL.Query().Get("v")

		if client != nil {
			//if _, err := client.Set(k, v, time.Minute).Result(); err == nil {
			if _, err := client.Set(k, v, time.Hour).Result(); err == nil {
				w.Write([]byte(fmt.Sprintf("set success k=%v, v=%v", k, v)))
			} else {
				w.Write([]byte(fmt.Sprintf("set failure k=%v, v=%v", k, v)))
			}
		} else {
			w.Write([]byte(fmt.Sprintf("redis disconnected")))
		}
	}

	getFile := func(filename string) ([]byte, error) {
		contents, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Printf("Failed to read file %s,err:%s", filename, err.Error())
			return nil, errors.New(fmt.Sprintf("Failed to read file %s,err:%s", filename, err.Error()))
		}
		return contents, nil
	}

	chatRoom := func(w http.ResponseWriter, req *http.Request) {
		if contents, err := getFile("chat192.html"); err != nil {
			w.Write([]byte("chat.html not found"))
			return
		} else {
			w.Write(contents)
		}
	}

	router := mux.NewRouter()

	router.HandleFunc("/hello", hello)
	router.HandleFunc("/headers", headers)

	router.HandleFunc("/get", get)
	router.HandleFunc("/ttl", ttl)
	router.HandleFunc("/set", set)

	router.HandleFunc("/chat", chatRoom)

	//启动http服务
	if err := http.ListenAndServe("192.168.2.21:8088", router); err != nil {
		fmt.Println("err:", err)
	}

	/*
		if err := http.ListenAndServe("192.168.2.21:8088", nil); err != nil {
			fmt.Printf("server failed to start err=%v\n", err)
		}*/
}

var client *redis.Client

func init() {
	// new redis client
	client = redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		//DB:       10,
	})

	// test connection
	_, err := client.Ping().Result()
	if err != nil {
		//log.Fatal(err)
		fmt.Printf("redis connection creation failed\n")
		client = nil
	} else {
		fmt.Printf("redis connection established\n")
		//// return pong if server is online
		//fmt.Println(pong)
	}
}

func createAInRedis() {
	if client == nil {
		fmt.Printf("redis a var set op failed due to conn failure\n")
		return
	}
	client.Set("a", "hello world", time.Second*3600*2)
}

func testRedis() bool {
	if client == nil {
		fmt.Printf("redis a var get op failed due to conn failure\n")
		return false
	}

	// get value
	_, err := client.Get("a").Result()
	if err != nil {
		fmt.Printf("failed to get a from redis client: %v", client)
		return false
	} else {
		return true
	}
}

func testMouseMove(ctx context.Context) <-chan struct{} {
	stopCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(stopCh)
			default:
			}
			robotgo.MoveMouseSmooth(1800, 900)
			robotgo.MoveMouseSmooth(200, 200)
		}
	}()

	return stopCh
}

func testRedisWithContextPractice() {
	fmt.Printf("redis conn: %v\n", client)
	createAInRedis()

	ctx, ctxCancel := context.WithCancel(context.Background())

	// access redis periodically
	go func() {
		for testRedis() {
			time.Sleep(time.Millisecond * 100)
		}

		fmt.Printf("testRedisWithContextPractice teminated for testRedis-loop\n")

		// cancel
		ctxCancel()
	}()

	// wait permanently here
	<-testMouseMove(ctx)
}

func testRateLimiting() {
	begin := time.Now()

	r := rate.Every(time.Millisecond * 50)
	initial := 10
	l := rate.NewLimiter(r, initial)

	var wg sync.WaitGroup
	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func(idx int) {
			ctx, _ := context.WithTimeout(context.Background(), 1*time.Second)
			if err := l.Wait(ctx); err == nil { //l.WaitN(ctx, 2)
				fmt.Printf("Access Permitted for #%-5d at %v\n", idx, time.Now())
			} else {
				fmt.Printf("Error %v for #%-5d at %v\n", err, idx, time.Now())
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
	fmt.Printf("rate limiting test completed, comsumed %v ms\n", time.Now().Sub(begin).Milliseconds())
}

func testRateLimitingForTokenInterval() {
	r := rate.Every(time.Millisecond * 1000)
	l := rate.NewLimiter(r, 5)

	for {
		ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)
		if err := l.Wait(ctx); err != nil {
			break
		}

		go func() {
			time.Sleep(time.Millisecond * 500)
			if !l.Allow() {
				fmt.Printf("Access not allowed at %v\n", time.Now())
			}
		}()

		fmt.Printf("Access Permitted at %v\n", time.Now())
	}
}

func testRateLimitingForBurstTraffic() {
	r := rate.Every(time.Millisecond * 5000)
	l := rate.NewLimiter(r, 9)

	for {
		if !l.Allow() {
			fmt.Printf("Access Denied at %v, tokens = %f\n", time.Now(), l.Tokens())
			<-time.After(time.Millisecond * 1000)
		} else {
			fmt.Printf("Access Permitted at %v, tokens = %f\n", time.Now(), l.Tokens())
		}
	}
}

func testContext() {
	// deadline test
	{
		ctx := context.Background()
		fmt.Printf("now = %v\n", time.Now())
		ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Hour))
		if deadline, ok := ctx.Deadline(); !ok {
			fmt.Printf("no deadline for context %v", ctx)
		} else {
			fmt.Printf("deadline = %v\n", deadline)
		}
	}

	// value test
	ctx := context.Background()
	key := "hello"
	subCtx := context.WithValue(ctx, key, "world")
	if val, ok := subCtx.Value(key).(string); ok {
		fmt.Printf("value for key = %v is %v\n", key, val)
	} else {
		fmt.Printf("no value for key = %v\n", key)
	}
}

func main() {
	// testReadClosedChannel()
	// testContextWithTimeout()
	// testContextWithDeadline()
	// testContextWithCancel()

	//testRedisWithContextPractice()

	//testRateLimiting()
	//testRateLimitingForTokenInterval()
	//testRateLimitingForBurstTraffic()

	testContext()

	//switch "chat" {
	switch "nothing" {
	case "http":
		testHttp()
	case "chat":
		go func() {
			testHttp()
		}()

		chatMain()
	}

}
