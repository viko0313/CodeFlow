package bus

import (
"sync"
)

type EventBus struct {
subscribers map[string][]chan interface{}
mu          sync.RWMutex
}

func NewEventBus() *EventBus {
return &EventBus{
subscribers: make(map[string][]chan interface{}),
}
}

func (eb *EventBus) Subscribe(eventType string, ch chan interface{}) {
eb.mu.Lock()
defer eb.mu.Unlock()
eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
}

func (eb *EventBus) Unsubscribe(eventType string, ch chan interface{}) {
eb.mu.Lock()
defer eb.mu.Unlock()
chans := eb.subscribers[eventType]
for i, c := range chans {
if c == ch {
eb.subscribers[eventType] = append(chans[:i], chans[i+1:]...)
break
}
}
}

func (eb *EventBus) Publish(eventType string, data interface{}) {
eb.mu.RLock()
defer eb.mu.RUnlock()
chans := eb.subscribers[eventType]
for _, ch := range chans {
go func(c chan interface{}) {
c <- data
}(ch)
}
}

type TaskMessage struct {
Type      string
Content   string
Priority  int
Timestamp int64
}

var taskQueue = make(chan TaskMessage, 100)

func EmitTask(msg TaskMessage) {
taskQueue <- msg
}

func GetTaskQueue() <-chan TaskMessage {
return taskQueue
}
