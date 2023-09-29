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

	if flag.NArg() != 1 {
		exit(fmt.Errorf("need filename as argument"))
	}
	file := flag.Arg(0)

	c := asm.NewCompiler(os.DirFS("."))
	c.SetUsePush0(!*noPush0)
	bin := c.CompileFile(file)
	if len(c.Errors()) > 0 {
		for _, err := range c.Errors() {
			fmt.Fprintln(os.Stderr, err)
		}
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
