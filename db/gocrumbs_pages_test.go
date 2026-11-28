package db

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	_ "github.com/lib/pq"

	"gopherdebt/models"
)

// testDB connects to GOPHERDEBT_TEST_DATABASE_URL and wipes it. The test is skipped without it,
// and refuses any database whose name doesn't end in _test, so it can never touch real data.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("GOPHERDEBT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("GOPHERDEBT_TEST_DATABASE_URL not set")
	}
	d, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	var name string
	if err := d.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatalf("refusing to wipe database %q: its name must end in _test", name)
	}
	if _, err := d.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func testUser(t *testing.T, d *sql.DB, n int) int {
	t.Helper()
	u, err := CreateUser(d, fmt.Sprintf("user%d@example.com", n), "hash", fmt.Sprintf("User %d", n))
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func testPage(name, work_type string) *models.GoCrumbsPageRequest {
	return &models.GoCrumbsPageRequest{
		Name: name,
		GoCrumbsRequest: models.GoCrumbsRequest{
			WorkType: work_type, GrossAmount: 4000, GrossPeriod: "month", SteuerKlasse: 1,
			Bundesland: "NW", Zusatzbeitrag: 2.9, Rent: 900, WsHoursPeriod: "month",
			Subscriptions:  []models.GoCrumbsExpenseLine{{Name: "Netflix", Amount: 12.99, Category: "media"}},
			CustomExpenses: []models.GoCrumbsExpenseLine{{Name: "Phone", Amount: 20}},
			ExtraPay:       []models.GoCrumbsExtraPayLine{{Name: "On-call", Amount: 500}},
		},
	}
}

// Existing single profiles become one "My budget" page each, exactly once, and the old tables
// keep their data
func TestCopyGoCrumbsProfilesToPages(t *testing.T) {
	d := testDB(t)
	alice, bob, carol := testUser(t, d, 1), testUser(t, d, 2), testUser(t, d, 3)

	// Profiles saved before pages existed (the flag is cleared to replay the first deploy)
	alice_req := testPage("", "fte").GoCrumbsRequest
	alice_req.Subscriptions = append(alice_req.Subscriptions, models.GoCrumbsExpenseLine{Name: "Spotify", Amount: 10.99})
	bob_req := testPage("", "werkstudent").GoCrumbsRequest
	bob_req.WsHourlyWage, bob_req.WsHours, bob_req.WsHoursPeriod = 17, 20, "week"
	bob_req.ExtraPay = []models.GoCrumbsExtraPayLine{}
	for _, p := range []struct {
		user int
		req  models.GoCrumbsRequest
	}{{alice, alice_req}, {bob, bob_req}} {
		if _, err := SaveGoCrumbsProfile(d, p.user, &p.req); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Exec(`DELETE FROM gocrumbs_pages; DELETE FROM migration_flags`); err != nil {
		t.Fatal(err)
	}
	if err := copyGoCrumbsProfilesToPages(d); err != nil {
		t.Fatal(err)
	}

	for _, user := range []int{alice, bob} {
		old, err := GetGoCrumbsProfile(d, user)
		if err != nil {
			t.Fatal(err)
		}
		list, err := ListGoCrumbsPages(d, user)
		if err != nil || len(list) != 1 || list[0].Name != "My budget" || list[0].WorkType != old.WorkType {
			t.Fatalf("user %d: pages %+v (%v)", user, list, err)
		}
		page, err := GetGoCrumbsPage(d, user, list[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(page.GoCrumbsRequest, old.GoCrumbsRequest) {
			t.Errorf("user %d: copy differs\n got  %+v\n want %+v", user, page.GoCrumbsRequest, old.GoCrumbsRequest)
		}
		if !page.UpdatedAt.Equal(old.UpdatedAt) {
			t.Errorf("user %d: updated_at %v, want %v", user, page.UpdatedAt, old.UpdatedAt)
		}
	}
	if list, _ := ListGoCrumbsPages(d, carol); len(list) != 0 {
		t.Errorf("carol had no profile but got pages %+v", list)
	}

	// Never again: not on a rerun, and not after the user deleted every page
	if err := copyGoCrumbsProfilesToPages(d); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`DELETE FROM gocrumbs_pages WHERE user_id = $1`, alice); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(d); err != nil {
		t.Fatal(err)
	}
	var pages, old_rows int
	d.QueryRow(`SELECT COUNT(*) FROM gocrumbs_pages`).Scan(&pages)
	d.QueryRow(`SELECT COUNT(*) FROM gocrumbs_profiles`).Scan(&old_rows)
	if pages != 1 || old_rows != 2 {
		t.Errorf("after rerun: %d pages (want 1), %d old profiles (want 2)", pages, old_rows)
	}
}

func TestGoCrumbsPagesCRUD(t *testing.T) {
	d := testDB(t)
	alice, bob := testUser(t, d, 1), testUser(t, d, 2)

	jan, err := CreateGoCrumbsPage(d, alice, testPage("Jan", "fte"))
	if err != nil {
		t.Fatal(err)
	}
	if jan.Name != "Jan" || len(jan.Subscriptions) != 1 || len(jan.CustomExpenses) != 1 || len(jan.ExtraPay) != 1 {
		t.Errorf("created page %+v", jan)
	}

	// Same name in the same work type is taken (ignoring case), in the other one it's free
	if _, err := CreateGoCrumbsPage(d, alice, testPage("JAN", "fte")); err != ErrDuplicate {
		t.Errorf("duplicate name: %v, want ErrDuplicate", err)
	}
	if _, err := CreateGoCrumbsPage(d, alice, testPage("Jan", "werkstudent")); err != nil {
		t.Errorf("same name, other work type: %v", err)
	}

	// Save replaces inputs and lines and can rename
	upd := testPage("January", "fte")
	upd.Rent = 950
	upd.Subscriptions = []models.GoCrumbsExpenseLine{}
	got, err := UpdateGoCrumbsPage(d, alice, jan.ID, upd)
	if err != nil || got.Name != "January" || got.Rent != 950 || len(got.Subscriptions) != 0 || len(got.CustomExpenses) != 1 {
		t.Errorf("update: %+v (%v)", got, err)
	}
	if _, err := UpdateGoCrumbsPage(d, alice, jan.ID, testPage("January", "werkstudent")); err != ErrGoCrumbsWorkTypeLocked {
		t.Errorf("work type change: %v, want ErrGoCrumbsWorkTypeLocked", err)
	}
	feb, _ := CreateGoCrumbsPage(d, alice, testPage("Feb", "fte"))
	if _, err := UpdateGoCrumbsPage(d, alice, feb.ID, testPage("january", "fte")); err != ErrDuplicate {
		t.Errorf("rename to a taken name: %v, want ErrDuplicate", err)
	}

	// Another user's page doesn't exist for bob
	if _, err := GetGoCrumbsPage(d, bob, jan.ID); err != ErrNotFound {
		t.Errorf("bob get: %v", err)
	}
	if _, err := UpdateGoCrumbsPage(d, bob, jan.ID, testPage("Hijack", "fte")); err != ErrNotFound {
		t.Errorf("bob update: %v", err)
	}
	if err := DeleteGoCrumbsPage(d, bob, jan.ID); err != ErrNotFound {
		t.Errorf("bob delete: %v", err)
	}
	if list, _ := ListGoCrumbsPages(d, bob); len(list) != 0 {
		t.Errorf("bob sees %+v", list)
	}

	// Delete removes the page and its lines
	if err := DeleteGoCrumbsPage(d, alice, jan.ID); err != nil {
		t.Fatal(err)
	}
	var lines int
	d.QueryRow(`SELECT (SELECT COUNT(*) FROM gocrumbs_page_expenses WHERE page_id = $1) +
		(SELECT COUNT(*) FROM gocrumbs_page_extra_pay WHERE page_id = $1)`, jan.ID).Scan(&lines)
	if lines != 0 {
		t.Errorf("%d lines left after delete", lines)
	}
	if err := DeleteGoCrumbsPage(d, alice, jan.ID); err != ErrNotFound {
		t.Errorf("second delete: %v", err)
	}
}

// 15 per work type, also when many creates race
func TestGoCrumbsPageLimit(t *testing.T) {
	d := testDB(t)
	alice := testUser(t, d, 1)

	for i := range 10 {
		if _, err := CreateGoCrumbsPage(d, alice, testPage(fmt.Sprintf("fte %d", i), "fte")); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = CreateGoCrumbsPage(d, alice, testPage(fmt.Sprintf("race %d", i), "fte"))
		}()
	}
	wg.Wait()
	limited := 0
	for _, err := range errs {
		if err == ErrGoCrumbsPageLimit {
			limited++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM gocrumbs_pages WHERE user_id = $1 AND work_type = 'fte'`, alice).Scan(&count)
	if count != MaxGoCrumbsPages || limited != 15 {
		t.Errorf("%d fte pages (want %d), %d refused (want 15)", count, MaxGoCrumbsPages, limited)
	}

	// The Werkstudent side has its own 15
	for i := range MaxGoCrumbsPages {
		if _, err := CreateGoCrumbsPage(d, alice, testPage(fmt.Sprintf("ws %d", i), "werkstudent")); err != nil {
			t.Fatalf("werkstudent page %d: %v", i, err)
		}
	}
	if _, err := CreateGoCrumbsPage(d, alice, testPage("ws 16", "werkstudent")); err != ErrGoCrumbsPageLimit {
		t.Errorf("16th werkstudent page: %v", err)
	}
}
