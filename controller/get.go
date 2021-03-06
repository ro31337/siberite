package controller

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync/atomic"
)

var timeoutRegexp = regexp.MustCompile(`(t\=\d+)\/?`)

// Get handles GET command
// Command: GET <queue>
// Response:
// VALUE <queue> 0 <bytes>
// <data block>
// END
func (c *Controller) Get(input []string) error {
	var err error
	cmd := parseGetCommand(input)

	switch cmd.SubCommand {
	case "", "open":
		err = c.get(cmd)
	case "close":
		err = c.getClose(cmd)
	case "close/open":
		if err = c.getClose(cmd); err == nil {
			err = c.get(cmd)
		}
	case "abort":
		err = c.getAbort(cmd)
	case "peek":
		err = c.peek(cmd)
	default:
		err = errors.New("ERROR " + "Invalid command")
	}

	if err != nil {
		return err
	}
	fmt.Fprint(c.rw.Writer, "END\r\n")
	c.rw.Writer.Flush()
	return nil
}

func (c *Controller) get(cmd *Command) error {
	if c.currentItem != nil {
		return errors.New("CLIENT_ERROR " + "Close current item first")
	}

	q, err := c.repo.GetQueue(cmd.QueueName)
	if err != nil {
		log.Printf("Can't GetQueue %s: %s", cmd.QueueName, err.Error())
		return errors.New("SERVER_ERROR " + err.Error())
	}
	item, _ := q.Dequeue()
	if len(item.Value) > 0 {
		fmt.Fprintf(c.rw.Writer, "VALUE %s 0 %d\r\n", cmd.QueueName, len(item.Value))
		fmt.Fprintf(c.rw.Writer, "%s\r\n", item.Value)
	}
	if strings.Contains(cmd.SubCommand, "open") && len(item.Value) > 0 {
		c.setCurrentState(cmd, item)
		q.AddOpenTransactions(1)
	}
	atomic.AddUint64(&c.repo.Stats.CmdGet, 1)
	return nil
}

func (c *Controller) getClose(cmd *Command) error {
	q, err := c.repo.GetQueue(cmd.QueueName)
	if err != nil {
		log.Printf("Can't GetQueue %s: %s", cmd.QueueName, err.Error())
		return errors.New("SERVER_ERROR " + err.Error())
	}
	if c.currentItem != nil {
		q.AddOpenTransactions(-1)
		c.setCurrentState(nil, nil)
	}

	return nil
}

func (c *Controller) getAbort(cmd *Command) error {
	err := c.abort(cmd)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) abort(cmd *Command) error {
	if c.currentItem != nil {
		q, err := c.repo.GetQueue(cmd.QueueName)
		if err != nil {
			log.Printf("Can't GetQueue %s: %s", cmd.QueueName, err.Error())
			return errors.New("SERVER_ERROR " + err.Error())
		}
		err = q.Prepend(c.currentItem)
		if err != nil {
			return errors.New("SERVER_ERROR " + err.Error())
		}
		if c.currentItem != nil {
			q.AddOpenTransactions(-1)
			c.setCurrentState(nil, nil)
		}
	}
	return nil
}

func (c *Controller) peek(cmd *Command) error {
	q, err := c.repo.GetQueue(cmd.QueueName)
	if err != nil {
		log.Printf("Can't GetQueue %s: %s", cmd.QueueName, err.Error())
		return errors.New("SERVER_ERROR " + err.Error())
	}
	item, _ := q.Peek()
	if len(item.Value) > 0 {
		fmt.Fprintf(c.rw.Writer, "VALUE %s 0 %d\r\n", cmd.QueueName, len(item.Value))
		fmt.Fprintf(c.rw.Writer, "%s\r\n", item.Value)
	}
	atomic.AddUint64(&c.repo.Stats.CmdGet, 1)
	return nil
}

func parseGetCommand(input []string) *Command {
	cmd := &Command{Name: input[0], QueueName: input[1], SubCommand: ""}
	if strings.Contains(input[1], "t=") {
		input[1] = timeoutRegexp.ReplaceAllString(input[1], "")
	}
	if strings.Contains(input[1], "/") {
		tokens := strings.SplitN(input[1], "/", 2)
		cmd.QueueName = tokens[0]
		cmd.SubCommand = strings.Trim(tokens[1], "/")
	}
	return cmd
}
