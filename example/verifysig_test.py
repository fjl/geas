#!/usr/bin/env python3

import subprocess
import os
import sys

def test_valid_signature(bytecode: str):
    """Test with a valid signature."""
    message = b"Hello, World!"
    sig = sign_message(message)
    calldata = sig + message.hex()

    result = run_contract(bytecode, calldata)
    expected = "0x01"

    if result == expected:
        print("PASS: valid signature returns 1")
        return True
    else:
        print(f"FAIL: valid signature - expected {expected}, got {result}")
        return False


def test_wrong_message(bytecode: str):
    """Test with wrong message (signature doesn't match)."""
    message = b"Hello, World!"
    sig = sign_message(message)

    wrong_message = b"Wrong message!"
    calldata = sig + wrong_message.hex()

    result = run_contract(bytecode, calldata)
    expected = "0x00"

    if result == expected:
        print("PASS: wrong message returns 0")
        return True
    else:
        print(f"FAIL: wrong message - expected {expected}, got {result}")
        return False


def test_corrupted_signature(bytecode: str):
    """Test with corrupted signature."""
    message = b"Hello, World!"
    sig = sign_message(message)

    # Corrupt r by changing first byte
    corrupted_sig = "ff" + sig[2:]
    calldata = corrupted_sig + message.hex()

    result = run_contract(bytecode, calldata)
    expected = "0x00"

    if result == expected:
        print("PASS: corrupted signature returns 0")
        return True
    else:
        print(f"FAIL: corrupted signature - expected {expected}, got {result}")
        return False


def test_empty_message(bytecode: str):
    """Test with empty message (should fail - msgLen must be > 0)."""
    sig = sign_message(b"")
    calldata = sig

    result = run_contract(bytecode, calldata)
    expected = "0x00"

    if result == expected:
        print("PASS: empty message returns 0")
        return True
    else:
        print(f"FAIL: empty message - expected {expected}, got {result}")
        return False


def test_long_message(bytecode: str):
    """Test with a longer message."""
    message = b"This is a longer test message to verify that the decimal length encoding works correctly for multi-digit lengths!"
    sig = sign_message(message)
    calldata = sig + message.hex()

    result = run_contract(bytecode, calldata)
    expected = "0x01"

    if result == expected:
        print("PASS: long message returns 1")
        return True
    else:
        print(f"FAIL: long message - expected {expected}, got {result}")
        return False


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
ROOT_DIR = os.path.dirname(SCRIPT_DIR)

ETHKEY = ["go", "run", "github.com/ethereum/go-ethereum/cmd/ethkey@latest"]
EVM = ["go", "run", "github.com/ethereum/go-ethereum/cmd/evm@latest"]
GEAS = ["go", "run", "./cmd/geas"]

KEYFILE = os.path.join(SCRIPT_DIR, "testdata", "testkey.json")
PASSFILE = os.path.join(SCRIPT_DIR, "testdata", "password.txt")


def run_cmd(cmd, input_data=None):
    result = subprocess.run(cmd, capture_output=True, text=True, input=input_data, cwd=ROOT_DIR)
    if result.returncode != 0:
        print(f"Command failed: {' '.join(cmd)}")
        print(f"stderr: {result.stderr}")
        sys.exit(1)
    return result.stdout


def compile_contract(file):
    return run_cmd(GEAS + ["-a", file])


def run_contract(bytecode: str, calldata: str) -> str:
    result = run_cmd(EVM + ["run", "--codefile", "/dev/stdin", "--input", calldata], bytecode)
    return result.strip()


def sign_message(message: bytes) -> str:
    """Sign a message using ethkey and return signature hex (r||s||v format)."""
    import tempfile
    with tempfile.NamedTemporaryFile(mode='wb', delete=False) as f:
        f.write(message)
        msgfile = f.name

    try:
        output = run_cmd(ETHKEY + ["signmessage", "--msgfile", msgfile, "--passwordfile", PASSFILE, KEYFILE])
    finally:
        os.unlink(msgfile)

    # Parse signature from output: "Signature: <hex>"
    for line in output.split('\n'):
        if line.startswith('Signature:'):
            return line.split()[1]

    print(f"Could not find signature in output: {output}")
    sys.exit(1)


def main():
    print("Compiling contract...")
    bytecode = compile_contract("example/verifysig.eas")
    print("Bytecode length:", len(bytecode))

    tests = [
        test_valid_signature,
        test_wrong_message,
        test_corrupted_signature,
        test_empty_message,
        test_long_message,
    ]
    failed = 0
    for test in tests:
        if not test(bytecode):
            failed += 1
    sys.exit(0 if failed == 0 else 1)


if __name__ == "__main__":
    main()
