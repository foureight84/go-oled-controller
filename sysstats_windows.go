// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// +build windows

// Get system status from TypePerf (Windows edition)

package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Start TypePerf for the specified fields (counters), and feed the results to the specified channel.
func typeperf(interval uint, result chan []string, quit chan bool, fields []string) {
	defer close(result)

	fields = append(append(fields, "-si"), strconv.Itoa(int(interval)))
	cmd := exec.Command("TypePerf", fields...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Println("Failed to connect stdout for TypePerf: ", err)
		return
	} else if err := cmd.Start(); err != nil {
		log.Println("Failed to start TypePerf:", err)
		return
	}

	defer cmd.Process.Kill()

	reader := bufio.NewReader(stdout)

	// Read the first empty line and header.
	reader.ReadString('\n')
	reader.ReadString('\n')

	for {
		select {
		case <-quit:
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				log.Println("Read error from TypePerf:", err)
				return
			}

			result <- strings.Split(strings.TrimSuffix(line, "\r\n"), ",")
		}
	}
}

// Get system statistics at the specified interval, rounded to whole seconds.
// This will get the current CPU, memory, swap (page file), and disk usage in fractions (0.0-1.0)
func SystemStats(interval time.Duration, results chan []float64, quit chan bool) {
	defer close(results)

	tp := make(chan []string, 5)
	go typeperf(uint(interval.Seconds()), tp, quit, []string{
		`\Processor(_Total)\% Processor Time`,
		`\Memory\% Committed Bytes In Use`,
		`\Paging file(_Total)\% Usage`,
		`\PhysicalDisk(_Total)\% Disk Time`,
	})

	// System status will take a second to fill up. To avoid it feeling like lag, send an empty result directly.
	results <- make([]float64, 4)

	for {
		select {
		case fields, more := <-tp:
			if !more {
				return
			}
			values := make([]float64, 4)
			for i, field := range fields {
				if i != 0 { // First field is a timestamp
					value, err := strconv.ParseFloat(strings.TrimRight(strings.TrimLeft(field, `"`), `"`), 64)
					if err != nil {
						log.Printf("Failed to parse field %d in TypePerf data: '%s'\n", i, field)
						value = 0
					}
					values[i-1] = value / 100
					if *gArgs.debug {
						switch i {
						case 1:
							log.Println("CPU%: ", value)
						case 2:
							log.Println("Mem%: ", value)
						case 3:
							log.Println("Swap: ", value)
						case 4:
							log.Println("Disk: ", value)
						}
					}
				}
			}
			results <- values
		case <-time.After(10 * time.Second):
			log.Println("TypePerf read timed out")
			quit <- true // FIXME: This is not enough if the process has hung
		}
	}
}
