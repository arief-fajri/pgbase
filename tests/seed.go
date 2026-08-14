package tests

import (
	"fmt"
	"log"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/types"
)

// SeedTestData creates the standard test collections and records
// required by the test suite.
//
// It is safe to call multiple times - it will check if the data
// already exists before creating it.
func SeedTestData(app core.App) error {
	// Recreate seed data each time to ensure clean state.
	// This is safe because each operation checks if the
	// item already exists before creating it.

	log.Println("[seed] Seeding test data...")

	// ---------------------------------------------------------------
	// Collections
	// ---------------------------------------------------------------

	if err := seedCollection(app, func() *core.Collection {
		demo1 := core.NewBaseCollection("demo1")
		demo1.Id = "wsmn24bux7wo113"
		demo1.System = false
		demo1.Fields.Add(&core.TextField{Name: "text"})
		demo1.Fields.Add(&core.NumberField{Name: "number"})
		demo1.Fields.Add(&core.SelectField{Name: "select_one", Values: []string{"optionA", "optionB", "optionC"}})
		demo1.Fields.Add(&core.SelectField{Name: "select_many", Values: []string{"optionA", "optionB", "optionC"}, MaxSelect: 3})
		demo1.Fields.Add(&core.JSONField{Name: "json"})
		demo1.Fields.Add(&core.FileField{Name: "file_one", MaxSelect: 1})
		demo1.Fields.Add(&core.FileField{Name: "file_many", MaxSelect: 5})
		demo1.Fields.Add(&core.URLField{Name: "url"})
		demo1.Fields.Add(&core.EmailField{Name: "email"})
		demo1.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		demo1.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		demo1.ListRule = types.Pointer("")
		demo1.ViewRule = types.Pointer("")
		demo1.CreateRule = types.Pointer("")
		demo1.UpdateRule = types.Pointer("")
		demo1.DeleteRule = types.Pointer("")
		return demo1
	}); err != nil {
		return fmt.Errorf("failed to seed demo1: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		demo2 := core.NewBaseCollection("demo2")
		demo2.Id = "llvuca81nly1qls"
		demo2.System = false
		demo2.Fields.Add(&core.TextField{Name: "title", Required: true})
		demo2.ListRule = types.Pointer("")
		demo2.ViewRule = types.Pointer("")
		demo2.CreateRule = types.Pointer("")
		demo2.UpdateRule = types.Pointer("")
		demo2.DeleteRule = types.Pointer("")
		return demo2
	}); err != nil {
		return fmt.Errorf("failed to seed demo2: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		demo3 := core.NewBaseCollection("demo3")
		demo3.Id = "demo3"
		demo3.System = false
		demo3.ListRule = types.Pointer("")
		demo3.ViewRule = types.Pointer("")
		demo3.CreateRule = types.Pointer("")
		demo3.UpdateRule = types.Pointer("")
		demo3.DeleteRule = types.Pointer("")
		return demo3
	}); err != nil {
		return fmt.Errorf("failed to seed demo3: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		nologin := core.NewBaseCollection("nologin")
		nologin.Id = "pq21r1ot7ap3f1a"
		nologin.System = false
		nologin.Fields.Add(&core.TextField{Name: "title"})
		nologin.ListRule = types.Pointer("")
		nologin.ViewRule = nil
		nologin.CreateRule = nil
		nologin.UpdateRule = nil
		nologin.DeleteRule = nil
		return nologin
	}); err != nil {
		return fmt.Errorf("failed to seed nologin: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		clients := core.NewAuthCollection("clients", "clients")
		clients.Id = "v851q4r790rhknl"
		clients.System = false
		clients.ListRule = types.Pointer("")
		clients.ViewRule = types.Pointer("")
		clients.CreateRule = types.Pointer("")
		clients.UpdateRule = types.Pointer("")
		clients.DeleteRule = types.Pointer("")
		return clients
	}); err != nil {
		return fmt.Errorf("failed to seed clients: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		demo4 := core.NewBaseCollection("demo4")
		demo4.Id = "demo4_id"
		demo4.System = false
		demo4.Fields.Add(&core.TextField{Name: "title"})
		demo4.Fields.Add(&core.JSONField{Name: "json_object"})
		demo4.ListRule = types.Pointer("")
		demo4.ViewRule = types.Pointer("")
		demo4.CreateRule = types.Pointer("")
		demo4.UpdateRule = types.Pointer("")
		demo4.DeleteRule = types.Pointer("")
		return demo4
	}); err != nil {
		return fmt.Errorf("failed to seed demo4: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		demo5 := core.NewBaseCollection("demo5")
		demo5.Id = "demo5_id"
		demo5.System = false
		demo5.Fields.Add(&core.TextField{Name: "title"})
		demo5.ListRule = types.Pointer("")
		demo5.ViewRule = types.Pointer("")
		demo5.CreateRule = types.Pointer("")
		demo5.UpdateRule = types.Pointer("")
		demo5.DeleteRule = types.Pointer("")
		return demo5
	}); err != nil {
		return fmt.Errorf("failed to seed demo5: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		uniques := core.NewBaseCollection("uniques")
		uniques.Id = "uniques_id"
		uniques.System = false
		uniques.Fields.Add(&core.TextField{Name: "unique_text"})
		uniques.ListRule = types.Pointer("")
		uniques.ViewRule = types.Pointer("")
		return uniques
	}); err != nil {
		return fmt.Errorf("failed to seed uniques: %w", err)
	}

	// ---------------------------------------------------------------
	// View collections
	// ---------------------------------------------------------------

	if err := seedCollection(app, func() *core.Collection {
		view1 := core.NewViewCollection("view1")
		view1.Id = "view1_id"
		view1.ViewQuery = "select id, text, cast(true as boolean) as bool, file_one from demo1 where id = '84nmscqy84lsi1t'"
		view1.ListRule = types.Pointer("@request.auth.id != ''")
		view1.ViewRule = types.Pointer("@request.auth.id != ''")
		return view1
	}); err != nil {
		return fmt.Errorf("failed to seed view1: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		view2 := core.NewViewCollection("view2")
		view2.Id = "view2_id"
		view2.ViewQuery = "select id, file_one, file_many from demo1"
		view2.ListRule = types.Pointer("")
		view2.ViewRule = types.Pointer("")
		return view2
	}); err != nil {
		return fmt.Errorf("failed to seed view2: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		numericIdView := core.NewViewCollection("numeric_id_view")
		numericIdView.Id = "numeric_id_view_id"
		numericIdView.ViewQuery = "select id, text from demo1 limit 2"
		numericIdView.ListRule = types.Pointer("")
		numericIdView.ViewRule = types.Pointer("")
		return numericIdView
	}); err != nil {
		return fmt.Errorf("failed to seed numeric_id_view: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		users := core.NewAuthCollection("users", "_pb_users_auth_")
		users.System = false
		users.ListRule = types.Pointer("id = @request.auth.id")
		users.ViewRule = types.Pointer("id = @request.auth.id")
		users.CreateRule = types.Pointer("")
		users.UpdateRule = types.Pointer("id = @request.auth.id")
		users.DeleteRule = types.Pointer("id = @request.auth.id")
		users.Fields.Add(&core.TextField{Name: "name", Max: 255})
		users.Fields.Add(&core.FileField{Name: "avatar", MaxSelect: 1, MimeTypes: []string{"image/jpeg", "image/png", "image/svg+xml", "image/gif", "image/webp"}})
		users.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		users.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		users.OAuth2.MappedFields.Name = "name"
		users.OAuth2.MappedFields.AvatarURL = "avatar"
		return users
	}); err != nil {
		return fmt.Errorf("failed to seed users: %w", err)
	}

	// ---------------------------------------------------------------
	// Auth records
	// ---------------------------------------------------------------

	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return fmt.Errorf("failed to find superusers collection: %w", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("failed to find users collection: %w", err)
	}

	clients, err := app.FindCollectionByNameOrId("clients")
	if err != nil {
		return fmt.Errorf("failed to find clients collection: %w", err)
	}

	authRecords := []struct {
		col      *core.Collection
		id       string
		email    string
		password string
	}{
		{superusers, "sywbhecnh46rhm0", "test@example.com", "1234567890"},
		{superusers, "sbmbsdb40jyxf7h", "test2@example.com", "1234567890"},
		{superusers, "9q2trqumvlyr3bd", "test3@example.com", "1234567890"},
		{users, "4q1xlclmfloku33", "test@example.com", "1234567890"},
		{users, "oap640cot4yru2s", "test2@example.com", "1234567890"},
		{users, "bgs820n361vj1qd", "test3@example.com", "1234567890"},
		{clients, "gk390qegs4y47wn", "test@example.com", "1234567890"},
	}

	for _, ar := range authRecords {
		existing, _ := app.FindAuthRecordByEmail(ar.col.Name, ar.email)
		if existing != nil {
			continue
		}
		record := core.NewRecord(ar.col)
		record.Id = ar.id
		record.Set("email", ar.email)
		record.Set("password", ar.password)
		record.Set("passwordConfirm", ar.password)
		if err := app.Save(record); err != nil {
			return fmt.Errorf("failed to create auth record %s/%s: %w", ar.col.Name, ar.id, err)
		}
	}

	// ---------------------------------------------------------------
	// Base records
	// ---------------------------------------------------------------

	demo1, _ := app.FindCollectionByNameOrId("demo1")
	demo2, _ := app.FindCollectionByNameOrId("demo2")
	nologin, _ := app.FindCollectionByNameOrId("nologin")
	demo4, _ := app.FindCollectionByNameOrId("demo4")

	baseRecords := []struct {
		col  *core.Collection
		id   string
		data map[string]any
	}{
		{demo1, "84nmscqy84lsi1t", map[string]any{"text": "test", "number": 123, "select_one": "optionA"}},
		{demo1, "al1h9ijdeojtsjy", map[string]any{"text": "test2", "number": 456, "select_one": "optionB"}},
		{demo1, "imy661ixudk5izi", map[string]any{"text": "test3", "number": 789}},
		{demo2, "0yxhwia2amd8gec", map[string]any{"title": "test_title"}},
		{demo2, "achvryl401bhse3", map[string]any{"title": "test_title2"}},
		{		demo2, "k7l9m3n4o5p6q7r", map[string]any{"title": "test_title3"}},
		{nologin, "dc49k6jgejn40h3", map[string]any{"title": "nologin1"}},
		{nologin, "oos036e9xvqeexy", map[string]any{"title": "nologin2"}},
		{nologin, "phhq3wr65cap535", map[string]any{"title": "nologin3"}},
		{demo4, "la4y2w4o98acwuj", map[string]any{"title": "demo4_title", "json_object": `{"a": {"b": "test"}}`}},
		{demo4, "qjeql998mtp1azp", map[string]any{"title": "demo4_title2"}},
	}

	for _, r := range baseRecords {
		if r.col == nil {
			continue
		}
		existing, _ := app.FindRecordById(r.col, r.id)
		if existing != nil {
			continue
		}
		record := core.NewRecord(r.col)
		record.Id = r.id
		for k, v := range r.data {
			record.Set(k, v)
		}
		if err := app.Save(record); err != nil {
			return fmt.Errorf("failed to create record %s/%s: %w", r.col.Name, r.id, err)
		}
	}

	// update demo1 record with file data (bypass validation)
	demo1Record, _ := app.FindRecordById(demo1, "84nmscqy84lsi1t")
	if demo1Record != nil {
		demo1Record.SetRaw("file_one", "test_d61b33QdDU.txt")
		demo1Record.SetRaw("file_many", `["test_QZFjKjXchk.txt"]`)
		app.SaveNoValidate(demo1Record)
	}

	// resolve collection references
	demo1, _ = app.FindCollectionByNameOrId("demo1")
	users, _ = app.FindCollectionByNameOrId("users")

	// add rel_many to demo1
	if demo1 != nil && users != nil {
		if demo1.Fields.GetByName("rel_many") == nil {
			demo1.Fields.Add(&core.RelationField{
				Name:         "rel_many",
				CollectionId: users.Id,
				MaxSelect:    5,
			})
			app.Save(demo1)
		}
	}

	log.Println("[seed] Test data seeded successfully")
	return nil
}

func seedCollection(app core.App, factory func() *core.Collection) error {
	col := factory()
	existing, _ := app.FindCollectionByNameOrId(col.Name)
	if existing != nil {
		return nil
	}
	// If the ID already exists under a different name (e.g. renamed by a test),
	// delete the stale collection first (only if it's not a system collection)
	existingById, _ := app.FindCollectionByNameOrId(col.Id)
	if existingById != nil && !existingById.System {
		app.Delete(existingById)
	}
	return app.Save(col)
}
