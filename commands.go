package vtui

// Стандартные идентификаторы команд, общие для всего фреймворка.
const (
	cmValid         = iota // 0
	cmQuit                 // Выход из приложения
	cmOK                   // Подтверждение (ОК)
	cmCancel               // Отмена
	cmYes                  // Да
	cmNo                   // Нет
	cmDefault              // Действие по умолчанию (Enter в диалоге)
	cmClose                // Закрыть окно
	cmZoom                 // Развернуть/свернуть окно (F5)
	cmResize               // Изменить размер
	cmNext                 // Следующее окно
	cmPrev                 // Предыдущее окно
	cmHelp                 // Вызов справки (F1)
	cmReceivedFocus        // Фрейм получил фокус
	cmReleasedFocus        // Фрейм потерял фокус
)