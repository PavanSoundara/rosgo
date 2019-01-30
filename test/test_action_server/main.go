package main

//go:generate gengo action actionlib_tutorials/Fibonacci Fibonacci.action
//go:generate gengo msg std_msgs/String
import (
        "fmt"
        "github.com/fetchrobotics/rosgo/ros"
        "os"
        "time"
        "actionlib_tutorials"
//      "std_msgs"
)

func callback(msg *actionlib_tutorials.FibonacciActionGoal) {
        fmt.Printf("Received: %s\n", msg.Goal.Order)
}

func main() {
        node, err := ros.NewNode("talker", os.Args)
        if err != nil {
                fmt.Println(err)
                os.Exit(-1)
        }
        defer node.Shutdown()
        node.Logger().SetSeverity(ros.LogLevelDebug)
	node.NewActionServer("fibonacci", actionlib_tutorials.ActionFibonacci, callback, false)

//	_ = node.NewPublisher("/batter", std_msgs.MsgString)

        for node.OK() {
                node.SpinOnce()
                time.Sleep(time.Second)
        }

        for node.OK() {
                node.SpinOnce()
                time.Sleep(time.Second)
        }
}
