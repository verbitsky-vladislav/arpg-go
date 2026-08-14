package mob

import "github.com/vladislav/game/internal/engine"

// Squad — общий штаб живых врагов на карте. Сам ничего не решает: он лишь то
// место, где враги договариваются о том, чего в одиночку не решить.
//
// Три вещи, которые нельзя вычислить, зная только себя:
//
//   - перекличка: увидел один — знают все, кто рядом, но с задержкой;
//   - очередь удара: бьют не все сразу, иначе толпа снимает игрока за секунду
//     и выглядит кашей, а не стаей;
//   - места окружения: каждому своё место в круге, чтобы не толкаться в одной
//     точке за спиной у соседа.
type Squad struct {
	members   []*Enemy
	attacking map[*Enemy]bool
	slotOf    map[*Enemy]int
	takenBy   map[int]*Enemy
	roleOf    map[*Enemy]Role
}

// Role — что особь делает в группе. Роль назначается один раз и держится:
// перебор ролей каждый тик выглядел бы как паника, а не как замысел.
type Role uint8

const (
	RoleNone   Role = iota
	RoleFront       // идёт в лоб и первым лезет в очередь удара
	RoleFlank       // заходит сбоку, занимая своё место в круге
	RoleCutoff      // встаёт на пути отхода: цель не должна просто убежать
)

var roleNames = [...]string{"none", "front", "flank", "cutoff"}

func (r Role) String() string {
	if int(r) >= len(roleNames) {
		return "?"
	}
	return roleNames[r]
}

// NewSquad создаёт пустой штаб.
func NewSquad() *Squad {
	return &Squad{
		attacking: map[*Enemy]bool{},
		slotOf:    map[*Enemy]int{},
		takenBy:   map[int]*Enemy{},
		roleOf:    map[*Enemy]Role{},
	}
}

// Add берёт особь под учёт.
func (s *Squad) Add(e *Enemy) { s.members = append(s.members, e) }

// Members — живой состав (в том числе трупы, пока они не растаяли: они всё ещё
// занимают место на земле и своих расталкивают).
func (s *Squad) Members() []*Enemy { return s.members }

// Prune убирает исчезнувших и освобождает их места и очередь удара.
func (s *Squad) Prune() {
	live := s.members[:0]
	for _, e := range s.members {
		if e.Gone() {
			s.forget(e)
			continue
		}
		if !e.Alive() {
			s.forget(e) // мёртвый очередь не занимает
		}
		live = append(live, e)
	}
	s.members = live
}

func (s *Squad) forget(e *Enemy) {
	delete(s.attacking, e)
	delete(s.roleOf, e)
	if slot, ok := s.slotOf[e]; ok {
		if s.takenBy[slot] == e {
			delete(s.takenBy, slot)
		}
		delete(s.slotOf, e)
	}
}

// Shout — крик того, кто заметил цель. Радиус меряется от кричащего, а не от
// цели: слышно того, кто орёт. Соседи узнают, где цель, но действовать
// начинают не мгновенно — у каждого своя задержка отклика, поэтому группа
// приходит волной, а не стеной.
func (s *Squad) Shout(from *Enemy, at engine.Vec2) {
	r := from.Bhv.Group.CallRadius
	for _, o := range s.members {
		if o == from || !o.Alive() || o.knows {
			continue
		}
		if engine.Dist(o.Pos, from.Pos) > r {
			continue
		}
		o.knows = true
		o.lastSeen = at
		o.memory = o.Bhv.Perception.MemoryTicks
		o.search = o.Bhv.Perception.SearchTicks
		o.react = max(o.react, o.Bhv.Group.CallDelayTicks)
		o.commit = 0 // пусть немедленно передумает, что делать
	}
}

// AssignRole выдаёт особи роль по составу группы: сначала набирается ударный
// строй, потом фланги, и лишь у большой группы появляется отрезающий — вдвоём
// перекрывать отход бессмысленно, это просто минус один боец.
func (s *Squad) AssignRole(e *Enemy) Role {
	if r, ok := s.roleOf[e]; ok {
		return r
	}
	var front, flank, cutoff, engaged int
	for _, o := range s.members {
		if !o.Alive() || !o.Engaged() {
			continue
		}
		engaged++
		switch s.roleOf[o] {
		case RoleFront:
			front++
		case RoleFlank:
			flank++
		case RoleCutoff:
			cutoff++
		}
	}
	r := RoleFront
	switch {
	case front < max(1, e.Bhv.Combat.AttackSlots):
		r = RoleFront
	case cutoff == 0 && engaged >= 4 && e.Bhv.Group.Cutoff > 0:
		r = RoleCutoff
	case e.flanker:
		r = RoleFlank
	}
	s.roleOf[e] = r
	return r
}

// Center — середина группы: от неё считается, с какой стороны группа пришла,
// а значит и куда цель побежит.
func (s *Squad) Center() engine.Vec2 {
	var sum engine.Vec2
	n := 0
	for _, o := range s.members {
		if !o.Alive() || !o.Engaged() {
			continue
		}
		sum = sum.Add(o.Pos)
		n++
	}
	if n == 0 {
		return engine.Vec2{}
	}
	return sum.Scale(1 / float64(n))
}

// RoleOf — роль особи (для отладки и тестов).
func (s *Squad) RoleOf(e *Enemy) Role { return s.roleOf[e] }

// ClaimAttack — просьба ударить. Разрешение даётся, пока бьющих меньше, чем
// позволяет профиль просящего: у гуманоидов строй шире, у зверей уже.
func (s *Squad) ClaimAttack(e *Enemy) bool {
	if s.attacking[e] {
		return true
	}
	if len(s.attacking) >= max(1, e.Bhv.Combat.AttackSlots) {
		return false
	}
	s.attacking[e] = true
	return true
}

// ReleaseAttack освобождает очередь после замаха.
func (s *Squad) ReleaseAttack(e *Enemy) { delete(s.attacking, e) }

// Attacking — сколько особей бьют прямо сейчас (для отладки и тестов).
func (s *Squad) Attacking() int { return len(s.attacking) }

// Slot выдаёт особи её место в круге окружения и держит его за ней: место,
// которое перевыдаётся каждый тик, заставляло бы врага метаться вокруг цели.
func (s *Squad) Slot(e *Enemy, slots int) int {
	if slot, ok := s.slotOf[e]; ok {
		return slot
	}
	if slots < 1 {
		return -1
	}
	for i := range slots {
		if s.takenBy[i] == nil {
			s.slotOf[e], s.takenBy[i] = i, e
			return i
		}
	}
	return -1
}
