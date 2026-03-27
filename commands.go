package vtui

// Стандартные идентификаторы команд, общие для всего фреймворка.
const (
	CmValid         = iota // 0
	CmQuit                 // Выход из приложения
	CmOK                   // Подтверждение (ОК)
	CmCancel               // Отмена
	CmYes                  // Да
	CmNo                   // Нет
	CmDefault              // Действие по умолчанию (Enter в диалоге)
	CmClose                // Закрыть окно
	CmZoom                 // Развернуть/свернуть окно (F5)
	CmResize               // Изменить размер
	CmNext                 // Следующее окно
	CmPrev                 // Предыдущее окно
	CmHelp                 // Вызов справки (F1)
	CmReceivedFocus        // Фрейм получил focus
	CmReleasedFocus        // Фрейм потерял focus

	// Application level standard commands
	CmCopy   = 100
	CmMove   = 101
	CmDelete = 102
	CmView   = 103
	CmEdit   = 104
	CmSearch = 105
	CmBackground = 106
	CmMkDir      = 107
)