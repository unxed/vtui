//go:build darwin && arm64

#include "textflag.h"

// This code is called via purego.SyscallN.
// It is ALREADY running on a large, safe C-stack.
// R0 contains 'im', R1-R7 contain the remaining arguments.

TEXT ·trampolineXGetIMValues(SB), NOSPLIT|NOFRAME, $0-0
    // Allocate 48 bytes (16-byte aligned)
    SUB $48, RSP, RSP
    // Save Frame Pointer (R29) and Link Register (R30)
    STP (R29, R30), 32(RSP)
    ADD $32, RSP, R29

    // Move R1-R3 to stack (Variadic arguments)
    MOVD R1, 0(RSP)
    MOVD R2, 8(RSP)
    MOVD R3, 16(RSP)

    // Read XGetIMValues function address from Go global variable
    MOVD ·xGetIMValuesPtr(SB), R12
    BL (R12)

    // Restore stack
    LDP 32(RSP), (R29, R30)
    ADD $48, RSP, RSP
    RET

TEXT ·trampolineXCreateIC(SB), NOSPLIT|NOFRAME, $0-0
    // Allocate 80 bytes (64 for 8 arguments + 16 for FP/LR)
    SUB $80, RSP, RSP
    STP (R29, R30), 64(RSP)
    ADD $64, RSP, R29

    // R0 (im) stays in register, R1-R7 go to stack
    MOVD R1, 0(RSP)
    MOVD R2, 8(RSP)
    MOVD R3, 16(RSP)
    MOVD R4, 24(RSP)
    MOVD R5, 32(RSP)
    MOVD R6, 40(RSP)
    MOVD R7, 48(RSP)

    // Read XCreateIC address
    MOVD ·xCreateICPtr(SB), R12
    BL (R12)

    LDP 64(RSP), (R29, R30)
    ADD $80, RSP, RSP
    RET
