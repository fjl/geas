// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/fjl/geas/asm"
)

func main() {
	var (
		binaryOutput = flag.Bool("bin", false, "binary output")
		noNL         = flag.Bool("no-nl", false, "remove newline at end of output")
		noPush0      = flag.Bool("no-push0", false, "disable use of PUSH0 instruction")
	)
	flag.Parse()

	var file string
	switch flag.NArg() {
	case 0:
		exit(fmt.Errorf("need filename as argument"))
	case 1:
		file = flag.Arg(0)
	default:
		exit(fmt.Errorf("too many arguments"))
	}
	if *noPush0 {
		exit(fmt.Errorf("option -no-push0 is not supported anymore"))
	}

	c := asm.NewCompiler(os.DirFS("."))
	bin := c.CompileFile(file)
	for _, err := range c.ErrorsAndWarnings() {
		fmt.Fprintln(os.Stderr, err)
	}
	if c.Failed() {
		os.Exit(1)
	}

	if *binaryOutput {
		os.Stdout.Write(bin)
	} else {
		os.Stdout.WriteString(hex.EncodeToString(bin))
		if !*noNL {
			os.Stdout.WriteString("\n")
		}
	}
}

func exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
