//go:build darwin && arm64

#include "textflag.h"

// Этот код будет вызван через purego.SyscallN.
// Он УЖЕ выполняется на большом безопасном C-стеке.
// R0 содержит 'im', R1-R7 содержат остальные аргументы.

TEXT ·trampolineXGetIMValues(SB), NOSPLIT|NOFRAME, $0-0
    // Выделяем 48 байт (16-byte aligned)
    SUB $48, RSP, RSP
    // Сохраняем Frame Pointer (R29) и Link Register (R30)
    STP (R29, R30), 32(RSP)
    ADD $32, RSP, R29

    // Перекладываем R1-R3 на стек (Variadic аргументы)
    MOVD R1, 0(RSP)
    MOVD R2, 8(RSP)
    MOVD R3, 16(RSP)

    // Читаем адрес функции XGetIMValues из глобальной переменной Go
    MOVD ·xGetIMValuesPtr(SB), R12
    BL (R12)

    // Восстанавливаем стек
    LDP 32(RSP), (R29, R30)
    ADD $48, RSP, RSP
    RET

TEXT ·trampolineXCreateIC(SB), NOSPLIT|NOFRAME, $0-0
    // Выделяем 80 байт (64 для 8 аргументов + 16 для FP/LR)
    SUB $80, RSP, RSP
    STP (R29, R30), 64(RSP)
    ADD $64, RSP, R29

    // R0 (im) остается на месте, R1-R7 идут на стек
    MOVD R1, 0(RSP)
    MOVD R2, 8(RSP)
    MOVD R3, 16(RSP)
    MOVD R4, 24(RSP)
    MOVD R5, 32(RSP)
    MOVD R6, 40(RSP)
    MOVD R7, 48(RSP)

    // Читаем адрес XCreateIC
    MOVD ·xCreateICPtr(SB), R12
    BL (R12)

    LDP 64(RSP), (R29, R30)
    ADD $80, RSP, RSP
    RET
