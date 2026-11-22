package db

import (
	"testing"
	"time"
)

// buildDemoData — чистый план демо-данных, поэтому его инварианты проверяются
// без MySQL (Б13, PLAN §9).
func TestBuildDemoData(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	data := buildDemoData(now)

	// 4 здания, из них 1 неактивное.
	if len(data.Buildings) != 4 {
		t.Fatalf("зданий = %d, want 4", len(data.Buildings))
	}
	inactive := 0
	for _, building := range data.Buildings {
		if !building.IsActive {
			inactive++
		}
	}
	if inactive != 1 {
		t.Fatalf("неактивных зданий = %d, want 1", inactive)
	}

	// 12 пользователей: 2 ADMIN без здания и 10 USER по активным зданиям.
	if len(data.Users) != 12 {
		t.Fatalf("пользователей = %d, want 12", len(data.Users))
	}
	admins, users := 0, 0
	for _, user := range data.Users {
		switch user.Role {
		case "ADMIN":
			admins++
			if user.BuildingIndex != -1 {
				t.Fatalf("ADMIN %q закреплён за зданием", user.Login)
			}
		case "USER":
			users++
			if user.BuildingIndex < 0 || user.BuildingIndex >= len(data.Buildings) {
				t.Fatalf("USER %q без корректного здания", user.Login)
			}
			if !data.Buildings[user.BuildingIndex].IsActive {
				t.Fatalf("USER %q в неактивном здании", user.Login)
			}
		default:
			t.Fatalf("неизвестная роль %q", user.Role)
		}
	}
	if admins != 2 || users != 10 {
		t.Fatalf("admins=%d users=%d, want 2/10", admins, users)
	}

	// 8 складов/кабинетов.
	if len(data.Warehouses) != 8 {
		t.Fatalf("складов = %d, want 8", len(data.Warehouses))
	}

	// 50 заявок: все статусы, done с completed_at, часть done — с просрочкой.
	if len(data.Tickets) != 50 {
		t.Fatalf("заявок = %d, want 50", len(data.Tickets))
	}
	statuses := map[string]int{}
	doneOverdue := 0
	for _, ticket := range data.Tickets {
		statuses[ticket.Status]++
		if ticket.BuildingIndex < 0 || ticket.BuildingIndex >= len(data.Buildings) {
			t.Fatalf("заявка без корректного здания: %+v", ticket)
		}
		if ticket.Status == "done" {
			if ticket.CompletedAt == nil {
				t.Fatalf("done без completed_at: %+v", ticket)
			}
			if ticket.Deadline != nil &&
				ticket.CompletedAt.After(ticket.Deadline.Add(24*time.Hour)) {
				doneOverdue++
			}
		} else if ticket.CompletedAt != nil {
			t.Fatalf("не-done с completed_at: %+v", ticket)
		}
	}
	for _, status := range []string{"new", "in_progress", "deferred", "done"} {
		if statuses[status] == 0 {
			t.Fatalf("нет заявок в статусе %q: %v", status, statuses)
		}
	}
	if statuses["done"] != 20 {
		t.Fatalf("done = %d, want 20", statuses["done"])
	}
	if doneOverdue == 0 {
		t.Fatal("среди done нет просроченных — дашборд не покажет просрочку")
	}

	// 25 единиц с уникальными регистрационными номерами и 16 перемещений.
	if len(data.Equipment) != 25 {
		t.Fatalf("оборудования = %d, want 25", len(data.Equipment))
	}
	regs := map[string]struct{}{}
	for _, item := range data.Equipment {
		if _, ok := regs[item.RegistrationNumber]; ok {
			t.Fatalf("дубль регистрационного номера %q", item.RegistrationNumber)
		}
		regs[item.RegistrationNumber] = struct{}{}
		if item.WarehouseIndex < 0 || item.WarehouseIndex >= len(data.Warehouses) {
			t.Fatalf("оборудование без корректного склада: %+v", item)
		}
	}
	if len(data.Movements) != 16 {
		t.Fatalf("перемещений = %d, want 16", len(data.Movements))
	}

	// 25 узлов БЗ, 2 скрытых корня, глубина ≥5, корректные ссылки на родителей.
	if len(data.Nodes) != 25 {
		t.Fatalf("узлов БЗ = %d, want 25", len(data.Nodes))
	}
	hiddenRoots := 0
	maxDepth := 0
	for i, node := range data.Nodes {
		if node.ParentIndex >= i {
			t.Fatalf("родитель узла %d должен идти раньше (parent=%d)", i, node.ParentIndex)
		}
		depth := 0
		for parent := node.ParentIndex; parent >= 0; parent = data.Nodes[parent].ParentIndex {
			depth++
			if depth > len(data.Nodes) {
				t.Fatalf("цикл в дереве БЗ у узла %d", i)
			}
		}
		if depth == 0 && node.HiddenForAnon {
			hiddenRoots++
		}
		if depth+1 > maxDepth {
			maxDepth = depth + 1
		}
	}
	if hiddenRoots != 2 {
		t.Fatalf("скрытых корней = %d, want 2", hiddenRoots)
	}
	if maxDepth < 5 {
		t.Fatalf("глубина дерева БЗ = %d, want ≥5", maxDepth)
	}

	// 5 праздников.
	if len(data.Holidays) != 5 {
		t.Fatalf("праздников = %d, want 5", len(data.Holidays))
	}
}
