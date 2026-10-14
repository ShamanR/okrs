package domain

// GoalDirection — направление цели: на какую область работы она нацелена. Набор
// закрыт; названия и описания направлений показывает клиент, здесь живут только коды.
type GoalDirection string

const (
	DirectionProduct GoalDirection = "PRODUCT"
	DirectionTech    GoalDirection = "TECH"
	DirectionProcess GoalDirection = "PROCESS"
)

// GoalDirections возвращает все направления в порядке показа: Продукт, Техника,
// Процессы. По этому порядку строятся балансы обзора периода.
func GoalDirections() []GoalDirection {
	return []GoalDirection{DirectionProduct, DirectionTech, DirectionProcess}
}

// Valid сообщает, входит ли значение в набор направлений. Пустое значение и прежние
// значения фокуса цели (PROFITABILITY и т. п.) недопустимы.
func (d GoalDirection) Valid() bool {
	switch d {
	case DirectionProduct, DirectionTech, DirectionProcess:
		return true
	}
	return false
}
