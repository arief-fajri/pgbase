package tests

import (
	"fmt"
	"log"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/types"
)

// seedTimeBase and seedTimeCounter implement a fixed monotonic seed clock.
//
// Seeded records normally get their "created" autodate value from time.Now(),
// which is stored with millisecond precision. Records saved within the same
// millisecond end up with identical "created" values and PostgreSQL breaks
// ORDER BY ties non-deterministically (unlike SQLite which falls back to the
// rowid insertion order), causing flaky paginated/sorted API tests.
// Assigning strictly increasing fixed timestamps makes the seed data
// ordering fully deterministic.
//
// The base is an arbitrary old date (no test compares seed "created" values
// against date boundaries) and dynamically created test records keep using
// now()-based values, which safely sort after the seeded ones in ascending
// order (and before them in descending order).
var seedTimeBase, _ = types.ParseDateTime("2022-01-01 00:00:00.000Z")
var seedTimeCounter int64

// nextSeedTime returns the next strictly increasing fixed timestamp
// (10ms apart from the previous one).
//
// Note: the clock is shared by the auth and base records seeding loops,
// so base records always get "newer" created values than auth records.
// This is currently harmless as no test sorts across collections by created.
func nextSeedTime() types.DateTime {
	seedTimeCounter++
	return seedTimeBase.Add(time.Duration(seedTimeCounter*10) * time.Millisecond)
}

// SeedTestData creates the standard test collections and records
// required by the test suite.
//
// It is safe to call multiple times - it will check if the data
// already exists before creating it.
func SeedTestData(app core.App) error {
	log.Println("[seed] Seeding test data...")

	// ---------------------------------------------------------------
	// Settings (match upstream test fixtures)
	// ---------------------------------------------------------------
	// Upstream test data enables the Batch API (enabled=true, maxRequests=50,
	// timeout=5). Persist it so cloned test apps pick it up on load.
	app.Settings().Batch.Enabled = true
	app.Settings().Batch.MaxRequests = 50
	app.Settings().Batch.Timeout = 5
	app.Settings().Batch.MaxBodySize = 0
	if err := app.Save(app.Settings()); err != nil {
		return fmt.Errorf("failed to seed settings: %w", err)
	}

	// ---------------------------------------------------------------
	// Collections
	// ---------------------------------------------------------------

	demo1Rule := types.Pointer("")

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewBaseCollection("demo1")
		c.Id = "wsmn24bux7wo113"
		c.System = false
		c.ListRule = nil
		c.ViewRule = nil
		c.CreateRule = nil
		c.UpdateRule = nil
		c.DeleteRule = nil
		c.Fields.Add(&core.TextField{Name: "text"})
		c.Fields.Add(&core.NumberField{Name: "number"})
		c.Fields.Add(&core.SelectField{Name: "select_one", Values: []string{"optionA", "optionB", "optionC"}})
		c.Fields.Add(&core.SelectField{Name: "select_many", Values: []string{"optionA", "optionB", "optionC"}, MaxSelect: 3})
		c.Fields.Add(&core.JSONField{Name: "json"})
		c.Fields.Add(&core.BoolField{Name: "bool"})
		c.Fields.Add(&core.FileField{Name: "file_one", MaxSelect: 1, Protected: true})
		c.Fields.Add(&core.FileField{Name: "file_many", MaxSelect: 5})
		c.Fields.Add(&core.URLField{Name: "url"})
		c.Fields.Add(&core.EmailField{Name: "email"})
		c.Fields.Add(&core.DateField{Name: "datetime"})
		c.Fields.Add(&core.AutodateField{Id: "_pbf_autodate_created_", Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Id: "_pbf_autodate_updated_", Name: "updated", OnCreate: true, OnUpdate: true})
		c.Fields.Add(&core.GeoPointField{Name: "point"})
		// align the auto-injected system id field id to the upstream fixture
		// (`_pbf_text_id_`) so system-field-change detection matches upstream;
		// our fork otherwise assigns a crc32-derived id (see memory).
		if idField := c.Fields.GetByName("id"); idField != nil {
			idField.SetId("_pbf_text_id_")
		}
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed demo1: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewBaseCollection("demo2")
		c.Id = "llvuca81nly1qls"
		c.System = false
		c.ListRule = demo1Rule
		c.ViewRule = demo1Rule
		c.CreateRule = demo1Rule
		c.UpdateRule = demo1Rule
		c.DeleteRule = demo1Rule
		c.Fields.Add(&core.TextField{Name: "title", Required: true, Min: 2})
		c.Fields.Add(&core.BoolField{Name: "active"})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		c.AddIndex("idx_unique_demo2_title", true, "title", "")
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed demo2: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewBaseCollection("demo3")
		c.Id = "wzlqyes4orhoygb"
		c.System = false
		// upstream demo3 rule: any authenticated non-users record (clients pass, users fail)
		demo3Rule := types.Pointer(`@request.auth.id != "" && @request.auth.collectionName != "users"`)
		c.ListRule = demo3Rule
		c.ViewRule = demo3Rule
		c.CreateRule = demo3Rule
		c.UpdateRule = demo3Rule
		c.DeleteRule = demo3Rule
		c.Fields.Add(&core.TextField{Name: "title"})
		c.Fields.Add(&core.FileField{Name: "files", MaxSelect: 99, MaxSize: 5242880})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed demo3: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewAuthCollection("nologin", "nologin")
		c.Id = "kpv709sk2lqbqk8"
		c.System = false
		// upstream nologin: all CRUD rules are public ("")
		c.ListRule = types.Pointer("")
		c.ViewRule = types.Pointer("")
		c.CreateRule = types.Pointer("")
		c.UpdateRule = types.Pointer("")
		c.DeleteRule = types.Pointer("")
		c.Fields.Add(&core.TextField{Name: "name"})
		// username is a custom (non-system) auth field in upstream v0.23 fixtures
		c.Fields.Add(&core.TextField{
			Id:                  "_pbf_auth_username_",
			Name:                "username",
			Required:            true,
			Min:                 3,
			Max:                 150,
			Pattern:             `^[\w][\w\.\-]*$`,
			AutogeneratePattern: "users[0-9]{5}",
		})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed nologin: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewAuthCollection("clients", "clients")
		c.Id = "v851q4r790rhknl"
		c.System = false
		// "only verified" behavior (matches the upstream clients collection);
		// must be set at creation so that a later AuthRule change does not
		// trigger the collection token secret rotation (see collection_model.go)
		c.AuthRule = types.Pointer("verified=true")
		c.ListRule = types.Pointer("")
		c.ViewRule = types.Pointer("")
		c.CreateRule = types.Pointer("")
		c.UpdateRule = types.Pointer("")
		c.DeleteRule = types.Pointer("")
		c.Fields.Add(&core.TextField{Name: "name"})
		// username is a custom (non-system) auth field in upstream v0.23 fixtures
		c.Fields.Add(&core.TextField{
			Id:                  "_pbf_auth_username_",
			Name:                "username",
			Required:            true,
			Min:                 3,
			Max:                 150,
			Pattern:             `^[\w][\w\.\-]*$`,
			AutogeneratePattern: "users[0-9]{5}",
		})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed clients: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewBaseCollection("demo4")
		c.Id = "demo4_id"
		c.System = false
		c.ListRule = types.Pointer("")
		c.ViewRule = types.Pointer("")
		c.CreateRule = types.Pointer("@request.auth.collectionName = 'users'")
		c.UpdateRule = types.Pointer("@request.auth.collectionName = 'users'")
		c.DeleteRule = types.Pointer("")
		c.Fields.Add(&core.TextField{Name: "title"})
		c.Fields.Add(&core.JSONField{Name: "json_object"})
		c.Fields.Add(&core.JSONField{Name: "json_array"})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed demo4: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewBaseCollection("demo5")
		c.Id = "9n89pl5vkct6330"
		c.System = false
		c.ListRule = types.Pointer("select_many:length = 3")
		// view rule references rel_many.self_rel_many.rel_many_cascade.files which
		// are wired after creation, so it is applied later (see demo5 view rule update).
		c.ViewRule = types.Pointer("")
		c.CreateRule = types.Pointer("@request.body.total = 3")
		c.UpdateRule = types.Pointer("@request.body.total = 3")
		c.DeleteRule = types.Pointer("@request.query.test:isset = true")
		c.Fields.Add(&core.SelectField{Name: "select_one", Values: []string{"a", "b", "c", "d"}, MaxSelect: 1})
		c.Fields.Add(&core.SelectField{Name: "select_many", Values: []string{"a", "b", "c", "d", "e"}, MaxSelect: 5})
		c.Fields.Add(&core.NumberField{Name: "total"})
		c.Fields.Add(&core.FileField{Name: "file"})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed demo5: %w", err)
	}

	// View collections are created later (see "View collections" section),
	// after demo1's rel_one/rel_many fields exist, because view1 selects them.

	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("failed to find users collection: %w", err)
	}

	// upstream users.email is NOT required (unlike clients/nologin/superusers)
	if emailField, ok := usersCol.Fields.GetByName("email").(*core.EmailField); ok {
		emailField.Required = false
	}
	// username is a custom (non-system) auth field in upstream v0.23 fixtures
	if usersCol.Fields.GetByName("username") == nil {
		usersCol.Fields.Add(&core.TextField{
			Id:                  "_pbf_auth_username_",
			Name:                "username",
			Required:            true,
			Min:                 3,
			Max:                 150,
			Pattern:             `^[\w][\w\.\-]*$`,
			AutogeneratePattern: "users[0-9]{5}",
		})
	}
	// upstream users collection also has a non-system "file" field (maxSelect 5)
	if usersCol.Fields.GetByName("file") == nil {
		usersCol.Fields.Add(&core.FileField{
			Id:        "xtecur3m",
			Name:      "file",
			MaxSelect: 5,
			MaxSize:   5242880,
		})
	}

	// align the avatar field thumbs with the pre-generated test data
	// (matches the upstream tests/data users collection avatar field)
	if avatarField, ok := usersCol.Fields.GetByName("avatar").(*core.FileField); ok {
		avatarField.Thumbs = []string{"70x50", "70x50t", "70x50b", "70x50f", "0x50", "70x0"}
		if err := app.Save(usersCol); err != nil {
			return fmt.Errorf("failed to set users avatar thumbs: %w", err)
		}
	}

	demo2, err := app.FindCollectionByNameOrId("demo2")
	if err != nil {
		return fmt.Errorf("failed to find demo2: %w", err)
	}

	// ---------------------------------------------------------------
	// Auth records
	// ---------------------------------------------------------------

	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return fmt.Errorf("failed to find superusers collection: %w", err)
	}

	nologinCol, err := app.FindCollectionByNameOrId("nologin")
	if err != nil {
		return fmt.Errorf("failed to find nologin: %w", err)
	}

	clients, err := app.FindCollectionByNameOrId("clients")
	if err != nil {
		return fmt.Errorf("failed to find clients: %w", err)
	}

	// ---------------------------------------------------------------
	// Auth collection options (match the upstream tests/data state)
	// ---------------------------------------------------------------

	setAuthOptions := func(col *core.Collection, mut func(c *core.Collection)) error {
		mut(col)
		if err := app.Save(col); err != nil {
			return fmt.Errorf("failed to set %s auth options: %w", col.Name, err)
		}
		return nil
	}

	// deterministic token secrets/durations matching the upstream
	// tests/data fixtures so that the hardcoded test tokens validate as-is
	const (
		testAuthTokenSecret                   = "PjVU4hAV7CZIWbCByJHkDcMUlSEWCLI6M5aWSZOpEq0a3rYxKT"
		testFileTokenSecret                   = "4Ax9zDm2Rwtny81dGaGQrJQBnIx5wVOuNe89X6v7NbNzrAZhvn"
		testVerificationTokenSecret           = "dgGGHlzzdCJ2C5MjXGoondllwSXkJHyL50FuvLvXGHNmBhvGKO"
		testPasswordResetTokenSecret          = "BC6jYPe4JXpQGGNzu6VXtYw0yhKoH2mh2ezIJClOJQuZYrd4Ol"
		testEmailChangeTokenSecret            = "eON2TTJZiGCEi7mvUvwMLADj8CMHQzwZN3gmyMjQb24EY08ATP"
		testSuperuserAuthTokenSecret          = "MyN3nDlzmHnuCjd35vb6cyIdqNr7Os0PmgiPVDMxmbFToSpBvS"
		testSuperuserFileTokenSecret          = "sjJAjTNPrOcRDmnIKwQm7qY9FyjuXTG5KNcaqw4U1TSDVfu4r9"
		testSuperuserVerificationTokenSecret  = "uhr68rXLVjPBWALFtw8uEHeQwDdN4t0MiTLr2pBWVkEQnNICe1"
		testSuperuserPasswordResetTokenSecret = "fPSpFm9rxjj4mdeWYfyQ5OZQ4UWpyainTO0dqrJe3LHEYEDduq"
		testSuperuserEmailChangeTokenSecret   = "unYNiYeuIxH7BCV09NIb81abe2bkPgaexMYdDQ6uOOIFh74urD"
	)

	setDefaultTokenOptions := func(c *core.Collection) {
		c.AuthToken = core.TokenConfig{
			Secret:   testAuthTokenSecret,
			Duration: 1209600,
		}
		c.PasswordResetToken = core.TokenConfig{
			Secret:   testPasswordResetTokenSecret,
			Duration: 1800,
		}
		c.EmailChangeToken = core.TokenConfig{
			Secret:   testEmailChangeTokenSecret,
			Duration: 1800,
		}
		c.VerificationToken = core.TokenConfig{
			Secret:   testVerificationTokenSecret,
			Duration: 604800,
		}
		c.FileToken = core.TokenConfig{
			Secret:   testFileTokenSecret,
			Duration: 180,
		}
	}

	if err := setAuthOptions(usersCol, func(c *core.Collection) {
		// the users collection is missing its functional username unique index
		// (upstream auth collections always have a case-insensitive one)
		if c.GetIndex("_pb_users_auth__username_idx") == "" {
			c.AddIndex("_pb_users_auth__username_idx", true, `LOWER("username")`, `"username" <> ''`)
		}
		setDefaultTokenOptions(c)
		c.PasswordAuth = core.PasswordAuthConfig{
			Enabled:        true,
			IdentityFields: []string{"email", "username"},
		}
		c.MFA = core.MFAConfig{
			Enabled:  true,
			Duration: 1800,
		}
		c.OTP.Enabled = true
		c.OTP.Duration = 300
		c.OTP.Length = 8
		c.OAuth2 = core.OAuth2Config{
			Enabled: true,
			MappedFields: core.OAuth2KnownFields{
				Username: "username",
			},
			Providers: []core.OAuth2ProviderConfig{
				{Name: "gitlab", ClientId: "test1", ClientSecret: "test2"},
				{Name: "google", ClientId: "test", ClientSecret: "test2"},
			},
		}
	}); err != nil {
		return err
	}

	if err := setAuthOptions(nologinCol, func(c *core.Collection) {
		setDefaultTokenOptions(c)
		c.PasswordAuth = core.PasswordAuthConfig{
			Enabled:        false,
			IdentityFields: []string{"email"},
		}
		c.OAuth2 = core.OAuth2Config{
			Enabled: false,
			Providers: []core.OAuth2ProviderConfig{
				{Name: "gitlab", ClientId: "test", ClientSecret: "test"},
			},
		}
		c.MFA = core.MFAConfig{
			Enabled:  false,
			Duration: 1800,
		}
		c.OTP.Enabled = false
		c.OTP.Duration = 300
		c.OTP.Length = 8
		c.ManageRule = types.Pointer("@request.auth.collectionName = 'users'")
	}); err != nil {
		return err
	}

	if err := setAuthOptions(clients, func(c *core.Collection) {
		// the clients collection is also missing its functional username unique index
		if c.GetIndex("idx_username_v851q4r790rhknl") == "" {
			c.AddIndex("idx_username_v851q4r790rhknl", true, `LOWER("username")`, `"username" <> ''`)
		}
		setDefaultTokenOptions(c)
		c.PasswordAuth = core.PasswordAuthConfig{
			Enabled:        true,
			IdentityFields: []string{"email", "username"},
		}
		// same value as at creation so no token secret rotation is triggered
		c.AuthRule = types.Pointer("verified=true")
	}); err != nil {
		return err
	}

	// superusers uses its own deterministic token secrets from the upstream fixtures
	superusers.AuthToken = core.TokenConfig{
		Secret:   testSuperuserAuthTokenSecret,
		Duration: 86400,
	}
	superusers.PasswordResetToken = core.TokenConfig{
		Secret:   testSuperuserPasswordResetTokenSecret,
		Duration: 1800,
	}
	superusers.EmailChangeToken = core.TokenConfig{
		Secret:   testSuperuserEmailChangeTokenSecret,
		Duration: 1800,
	}
	superusers.VerificationToken = core.TokenConfig{
		Secret:   testSuperuserVerificationTokenSecret,
		Duration: 259200,
	}
	superusers.FileToken = core.TokenConfig{
		Secret:   testSuperuserFileTokenSecret,
		Duration: 180,
	}
	if err := app.UnsafeWithoutHooks().Save(superusers); err != nil {
		return fmt.Errorf("failed to set superusers token options: %w", err)
	}

	authRecords := []struct {
		col      *core.Collection
		id       string
		email    string
		username string
		name     string
		password string
		tokenKey string
		verified bool
	}{
		{superusers, "sywbhecnh46rhm0", "test@example.com", "", "", "1234567890", "O4rvW9FSUyTA3xUuQmXR3wHF2db9bHs19nBHeSgVTxerOsTAl4", false},
		{superusers, "sbmbsdb40jyxf7h", "test2@example.com", "", "", "1234567890", "cvg1nk1dKRFlazQH8nCKuFYwczdReQx6ZJimxXvei0uDyTkgEb", false},
		{superusers, "9q2trqumvlyr3bd", "test3@example.com", "", "", "1234567890", "ezLvEu7DRFtUp9BI6nxtXCpgtp7qWaNQLdD6dDwjIVB0mA0uUr", false},
		{superusers, "5gx6b3yumzz6385", "test4@example.com", "", "", "1234567890", "J4L1xUuQmXR3wHF2db9bHs19nBHeSgVTxerOsTAl4B4LS", false},
		{usersCol, "4q1xlclmfloku33", "test@example.com", "users75657", "test1", "1234567890", "tfYe7rCTX4D2KuWQY3pJjBifgsrMbecyXBatEPjrSfGEGS2jh6", false},
		{usersCol, "oap640cot4yru2s", "test2@example.com", "test2_username", "", "1234567890", "AQbE30CNb8Ncwr6Sg0sfvDJGJuepriTJN24EHZqO5DsEBTk1kA", true},
		{usersCol, "bgs820n361vj1qd", "test3@example.com", "users69238", "test3", "1234567890", "x6vHUi00LvM5bFeGIpwXN9xuol8k1BknfTmlySQ7YQWoTLKOa7", true},
		{clients, "gk390qegs4y47wn", "test@example.com", "clients57772", "", "1234567890", "rMb1gUpn27s53t66gOGscSHfYsa272cgOgn4nhTZIl4fIC8XP8", true},
		{clients, "o1y0dd0spd786md", "test2@example.com", "clients43362", "test_name", "1234567890", "RunSD73nFfH3sNScreizPcZYNkiPls2YjmFYPNo73cWKsDnZVm", false},
		{nologinCol, "dc49k6jgejn40h3", "test@example.com", "test_username", "test", "1234567890", "6mi4JvnX8uIxS7JiO8LWl150Af8mnAGWkWRImHL2YB4XhOYf9c", false},
		{nologinCol, "oos036e9xvqeexy", "test3@example.com", "nologin84738", "", "1234567890", "uYafEN2cDgImHKMW4SfZ6tayrENAJVVWOTDxFXatOs2AO0KEMY", true},
		{nologinCol, "phhq3wr65cap535", "test2@example.com", "viewers74618", "", "1234567890", "9Bsj2ogZ5b0Q3daAKZ5ZZrUuVb7lWHO0YDD4d5TbPCLfYE3COY", false},
	}

	for _, ar := range authRecords {
		record, err0 := app.FindRecordById(ar.col, ar.id)
		isNew := err0 != nil
		if isNew {
			record = core.NewRecord(ar.col)
			record.Id = ar.id
		}
		record.Set("email", ar.email)
		if ar.username != "" {
			record.Set("username", ar.username)
		}
		if ar.name != "" {
			record.Set("name", ar.name)
		}
		record.Set("tokenKey", ar.tokenKey)
		record.Set("verified", ar.verified)
		// upstream: only these two auth records expose their email publicly
		if ar.id == "bgs820n361vj1qd" || ar.id == "phhq3wr65cap535" {
			record.Set("emailVisibility", true)
		}
		if isNew {
			record.Set("password", ar.password)
			record.Set("passwordConfirm", ar.password)
			// deterministic fixed "created" value (see nextSeedTime)
			record.SetRaw("created", nextSeedTime())
		}
		if err := app.Save(record); err != nil {
			return fmt.Errorf("failed to create auth record %s/%s: %w", ar.col.Name, ar.id, err)
		}
	}

	// ---------------------------------------------------------------
	// Base records
	// ---------------------------------------------------------------

	demo1, _ := app.FindCollectionByNameOrId("demo1")
	demo3, _ := app.FindCollectionByNameOrId("demo3")
	demo4, _ := app.FindCollectionByNameOrId("demo4")
	demo5, _ := app.FindCollectionByNameOrId("demo5")

	baseRecords := []struct {
		col  *core.Collection
		id   string
		data map[string]any
	}{
		{demo1, "84nmscqy84lsi1t", map[string]any{"text": "test", "number": 123456, "select_one": "optionB", "select_many": []string{"optionB", "optionC"}, "bool": true, "url": "https://example.copm", "email": "test@example.com", "datetime": "2022-10-01 12:00:00.000Z", "json": []any{1, 2, 3}}},
		{demo1, "al1h9ijdeojtsjy", map[string]any{"text": "test2", "number": 456, "select_one": "optionB", "select_many": []string{"optionB"}, "email": "test2@example.com"}},
		{demo1, "imy661ixudk5izi", map[string]any{"text": "lorem ipsum", "number": 0}},
		{demo2, "0yxhwia2amd8gec", map[string]any{"title": "test3", "active": true}},
		{demo2, "achvryl401bhse3", map[string]any{"title": "test2", "active": true}},
		{demo2, "k7l9m3n4o5p6q7r", map[string]any{"title": "test1"}},
		{demo3, "1tmknxy2868d869", map[string]any{"title": "test1"}},
		{demo3, "lcl9d87w22ml6jy", map[string]any{"title": "test2"}},
		{demo3, "7nwo8tuiatetxdm", map[string]any{"title": "test3"}},
		{demo3, "mk5fmymtx4wsprk", map[string]any{"title": "test4"}},
		{demo4, "i9naidtvr6qsgb4", map[string]any{"json_object": `{"a": {"b": "test"}}`}},
		{demo4, "qzaqccwrmva4o1n", map[string]any{"title": "test"}},
		{demo5, "qjeql998mtp1azp", map[string]any{"total": 0, "select_one": "b", "select_many": []string{"b", "c", "a"}}},
		{demo5, "la4y2w4o98acwuj", map[string]any{"total": 2}},
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
		// deterministic fixed "created" value in baseRecords slice order
		// (see nextSeedTime); in particular demo1 gets:
		// 84nmscqy84lsi1t < al1h9ijdeojtsjy < imy661ixudk5izi
		record.SetRaw("created", nextSeedTime())
		for k, v := range r.data {
			record.Set(k, v)
		}
		if err := app.Save(record); err != nil {
			return fmt.Errorf("failed to create record %s/%s: %w", r.col.Name, r.id, err)
		}
	}

	// ---------------------------------------------------------------
	// Relations wiring (users -> demo2, demo1 -> users,
	// demo4 -> demo3 + self, demo5 -> demo4)
	// ---------------------------------------------------------------

	if usersCol.Fields.GetByName("rel") == nil {
		// upstream users.rel is a SINGLE relation to demo2 (maxSelect 1)
		usersCol.Fields.Add(&core.RelationField{Name: "rel", CollectionId: demo2.Id, MaxSelect: 1})
		if err := app.Save(usersCol); err != nil {
			return fmt.Errorf("failed to add users.rel field: %w", err)
		}
	}
	if demo1.Fields.GetByName("rel_one") == nil {
		// upstream demo1.rel_one is a SELF relation to demo1 (not users), cascade false
		demo1.Fields.Add(&core.RelationField{Name: "rel_one", CollectionId: demo1.Id, MaxSelect: 1})
	}
	if demo1.Fields.GetByName("rel_many") == nil {
		// upstream demo1.rel_many -> users, cascade true
		demo1.Fields.Add(&core.RelationField{Name: "rel_many", CollectionId: usersCol.Id, MaxSelect: 9999, CascadeDelete: true})
	}
	if err := app.Save(demo1); err != nil {
		return fmt.Errorf("failed to add demo1 relation fields: %w", err)
	}

	// ---------------------------------------------------------------
	// View collections (created here so view1 can reference demo1's
	// rel_one/rel_many columns, and view2 can select from view1)
	// ---------------------------------------------------------------

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewViewCollection("view1")
		c.Id = "view1_id"
		c.ViewQuery = "select id, text, bool, url, select_one, select_many, file_one, file_many, number, email, datetime, json, rel_one, rel_many, point, created from demo1"
		c.ListRule = types.Pointer(`@request.auth.id != "" && bool = true`)
		c.ViewRule = types.Pointer(`@request.auth.id != "" && bool = true`)
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed view1: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewViewCollection("view2")
		c.Id = "view2_id"
		c.ViewQuery = "select view1.id, view1.bool as state, view1.file_many, view1.rel_many from view1"
		c.ListRule = types.Pointer("")
		c.ViewRule = types.Pointer("")
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed view2: %w", err)
	}

	if err := seedCollection(app, func() *core.Collection {
		c := core.NewViewCollection("numeric_id_view")
		c.Id = "numeric_id_view_id"
		c.ViewQuery = "select (ROW_NUMBER() OVER()) as id, email from clients"
		c.ListRule = types.Pointer("")
		c.ViewRule = types.Pointer("")
		return c
	}); err != nil {
		return fmt.Errorf("failed to seed numeric_id_view: %w", err)
	}

	if demo4 != nil && demo3 != nil {
		for _, rel := range []*core.RelationField{
			{Name: "rel_one_no_cascade", CollectionId: demo3.Id, MaxSelect: 1},
			{Name: "rel_one_no_cascade_required", CollectionId: demo3.Id, MaxSelect: 1, Required: true},
			{Name: "rel_one_cascade", CollectionId: demo3.Id, MaxSelect: 1, CascadeDelete: true},
			{Name: "rel_one_unique", CollectionId: demo3.Id, MaxSelect: 1},
			{Name: "rel_many_no_cascade", CollectionId: demo3.Id, MaxSelect: 5},
			{Name: "rel_many_no_cascade_required", CollectionId: demo3.Id, MaxSelect: 3, Required: true, MinSelect: 1},
			{Name: "rel_many_cascade", CollectionId: demo3.Id, MaxSelect: 5, CascadeDelete: true},
			{Name: "rel_many_unique", CollectionId: demo3.Id, MaxSelect: 5},
		} {
			if demo4.Fields.GetByName(rel.Name) == nil {
				demo4.Fields.Add(rel)
			}
		}
		if demo4.Fields.GetByName("self_rel_one") == nil {
			demo4.Fields.Add(&core.RelationField{Name: "self_rel_one", CollectionId: demo4.Id, MaxSelect: 1})
		}
		if demo4.Fields.GetByName("self_rel_many") == nil {
			demo4.Fields.Add(&core.RelationField{Name: "self_rel_many", CollectionId: demo4.Id, MaxSelect: 5})
		}
		if err := app.Save(demo4); err != nil {
			return fmt.Errorf("failed to save demo4 relation fields: %w", err)
		}
	}

	if demo5 != nil && demo4 != nil {
		if demo5.Fields.GetByName("rel_one") == nil {
			demo5.Fields.Add(&core.RelationField{Name: "rel_one", CollectionId: demo4.Id, MaxSelect: 1})
		}
		if demo5.Fields.GetByName("rel_many") == nil {
			demo5.Fields.Add(&core.RelationField{Name: "rel_many", CollectionId: demo4.Id, MaxSelect: 5})
		}
		// now that the demo5->demo4->demo4->demo3 relation chain exists, the
		// upstream view rule field path can be resolved
		demo5.ViewRule = types.Pointer("rel_many.self_rel_many.rel_many_cascade.files:length = 1")
		app.Save(demo5)
	}

	// rel values
	setRel := func(col *core.Collection, id string, pairs map[string]any) {
		if col == nil {
			return
		}
		// re-resolve the collection fresh to avoid any stale-cache issues
		if fresh, ferr := app.FindCollectionByNameOrId(col.Name); ferr == nil && fresh != nil {
			col = fresh
		}
		rec, _ := app.FindRecordById(col, id)
		if rec == nil {
			return
		}
		changed := false
		for k, v := range pairs {
			if cur := rec.Get(k); !stringifyEq(cur, v) {
				rec.Set(k, v)
				changed = true
			}
		}
		if changed {
			if sErr := app.SaveNoValidate(rec); sErr != nil {
				log.Printf("[seed] setRel %s/%s failed: %v", col.Name, id, sErr)
			}
		}
	}

	setRel(demo1, "84nmscqy84lsi1t", map[string]any{
		"rel_one":  "",
		"rel_many": []string{"oap640cot4yru2s"},
	})
	setRel(demo1, "al1h9ijdeojtsjy", map[string]any{
		"rel_one":  "84nmscqy84lsi1t",
		"rel_many": []string{"bgs820n361vj1qd", "4q1xlclmfloku33", "oap640cot4yru2s"},
	})
	setRel(demo4, "i9naidtvr6qsgb4", map[string]any{
		"rel_one_no_cascade_required": "lcl9d87w22ml6jy",
		"rel_one_cascade":             "7nwo8tuiatetxdm",
	})
	setRel(demo4, "qzaqccwrmva4o1n", map[string]any{
		"rel_one_no_cascade":           "mk5fmymtx4wsprk",
		"rel_one_no_cascade_required":  "lcl9d87w22ml6jy",
		"rel_one_cascade":              "mk5fmymtx4wsprk",
		"rel_many_no_cascade":          []string{"mk5fmymtx4wsprk"},
		"rel_many_no_cascade_required": []string{"7nwo8tuiatetxdm", "lcl9d87w22ml6jy"},
		"rel_many_cascade":             []string{"lcl9d87w22ml6jy"},
	})
	// raw SQL fallback for demo4 relations (FindRecordById + SaveNoValidate quirk)
	app.DB().NewQuery(
		`UPDATE "demo4" SET
			"rel_one_no_cascade_required" = 'lcl9d87w22ml6jy',
			"rel_one_cascade" = '7nwo8tuiatetxdm'
		WHERE "id" = 'i9naidtvr6qsgb4'
		AND ("rel_one_no_cascade_required" IS NULL OR "rel_one_no_cascade_required" = '')`,
	).Execute()
	app.DB().NewQuery(
		`UPDATE "demo4" SET
			"rel_one_no_cascade" = 'mk5fmymtx4wsprk',
			"rel_one_no_cascade_required" = 'lcl9d87w22ml6jy',
			"rel_one_cascade" = 'mk5fmymtx4wsprk',
			"rel_many_no_cascade" = '["mk5fmymtx4wsprk"]'::jsonb,
			"rel_many_no_cascade_required" = '["7nwo8tuiatetxdm","lcl9d87w22ml6jy"]'::jsonb,
			"rel_many_cascade" = '["lcl9d87w22ml6jy"]'::jsonb
		WHERE "id" = 'qzaqccwrmva4o1n'
		AND ("rel_one_no_cascade" IS NULL OR "rel_one_no_cascade" = '')`,
	).Execute()
	// raw SQL for canonical demo4 relation graph (expand tests)
	app.DB().NewQuery(
		`UPDATE "demo4" SET
			"self_rel_one" = 'i9naidtvr6qsgb4',
			"self_rel_many" = '["i9naidtvr6qsgb4","qzaqccwrmva4o1n"]'::jsonb,
			"rel_one_no_cascade" = 'mk5fmymtx4wsprk',
			"rel_one_no_cascade_required" = 'lcl9d87w22ml6jy',
			"rel_one_cascade" = '7nwo8tuiatetxdm',
			"rel_one_unique" = '',
			"rel_many_no_cascade" = '["1tmknxy2868d869"]'::jsonb,
			"rel_many_no_cascade_required" = '["lcl9d87w22ml6jy","1tmknxy2868d869"]'::jsonb,
			"rel_many_cascade" = '["mk5fmymtx4wsprk"]'::jsonb,
			"rel_many_unique" = '[]'::jsonb
		WHERE "id" = 'qzaqccwrmva4o1n'`,
	).Execute()
	app.DB().NewQuery(
		`UPDATE "demo4" SET
			"self_rel_one" = 'qzaqccwrmva4o1n',
			"self_rel_many" = '[]'::jsonb,
			"rel_one_no_cascade" = '',
			"rel_one_no_cascade_required" = 'lcl9d87w22ml6jy',
			"rel_one_cascade" = '',
			"rel_one_unique" = '',
			"rel_many_no_cascade" = '[]'::jsonb,
			"rel_many_no_cascade_required" = '["7nwo8tuiatetxdm"]'::jsonb,
			"rel_many_cascade" = '[]'::jsonb,
			"rel_many_unique" = '[]'::jsonb
		WHERE "id" = 'i9naidtvr6qsgb4'`,
	).Execute()
	setRel(demo5, "qjeql998mtp1azp", map[string]any{
		"rel_one":  "i9naidtvr6qsgb4",
		"rel_many": []string{"qzaqccwrmva4o1n", "i9naidtvr6qsgb4"},
	})
	// raw SQL fallback for demo5 relations (single -> scalar text, multi -> jsonb array)
	app.DB().NewQuery(
		`UPDATE "demo5" SET
			"rel_one" = 'i9naidtvr6qsgb4',
			"rel_many" = '["qzaqccwrmva4o1n","i9naidtvr6qsgb4"]'::jsonb
		WHERE "id" = 'qjeql998mtp1azp'`,
	).Execute()
	setRel(usersCol, "4q1xlclmfloku33", map[string]any{"rel": "k7l9m3n4o5p6q7r"})
	setRel(usersCol, "bgs820n361vj1qd", map[string]any{"rel": "0yxhwia2amd8gec"})
	// raw SQL fallback for users rel field (single relation -> scalar text)
	app.DB().NewQuery(
		`UPDATE "users" SET "rel" = 'k7l9m3n4o5p6q7r' WHERE "id" = '4q1xlclmfloku33'`,
	).Execute()
	app.DB().NewQuery(
		`UPDATE "users" SET "rel" = '0yxhwia2amd8gec' WHERE "id" = 'bgs820n361vj1qd'`,
	).Execute()
	// oap640cot4yru2s intentionally has no rel values (used as "no rels" record)

	// ---------------------------------------------------------------
	// Auth origins
	// ---------------------------------------------------------------

	originsCol, err := app.FindCollectionByNameOrId(core.CollectionNameAuthOrigins)
	if err != nil {
		return fmt.Errorf("failed to find authOrigins collection: %w", err)
	}

	origBase := types.NowDateTime().Add(-10 * time.Minute)
	authOrigins := []struct {
		id          string
		colRef      string
		recordRef   string
		fingerprint string
		created     types.DateTime
	}{
		{"5f29jy38bf5zm3f", superusers.Id, "sywbhecnh46rhm0", "5f29jy38bf5zm3f", origBase},
		{"dmy260k6ksjr4ib", superusers.Id, "sbmbsdb40jyxf7h", "dmy260k6ksjr4ib", origBase.Add(1 * time.Minute)},
		{"ic55o70g4f8pcl4", superusers.Id, "sbmbsdb40jyxf7h", "22bbbcbed36e25321f384ccf99f60057", origBase.Add(2 * time.Minute)},
		{"5798yh833k6w6w0", superusers.Id, "sbmbsdb40jyxf7h", "6afbfe481c31c08c55a746cccb88ece0", origBase.Add(3 * time.Minute)},
		{"9r2j0m74260ur8i", clients.Id, "gk390qegs4y47wn", "9r2j0m74260ur8i", origBase.Add(4 * time.Minute)},
	}

	for _, o := range authOrigins {
		if o.colRef == "" {
			continue
		}
		origin := core.NewRecord(originsCol)
		origin.Id = o.id
		origin.SetRaw("created", o.created)
		origin.Set("collectionRef", o.colRef)
		origin.Set("recordRef", o.recordRef)
		origin.Set("fingerprint", o.fingerprint)
		if err := app.Save(origin); err != nil {
			return fmt.Errorf("failed to create auth origin %s: %w", o.id, err)
		}
	}

	// ---------------------------------------------------------------
	// External auths
	// ---------------------------------------------------------------

	extAuthsCol, err := app.FindCollectionByNameOrId(core.CollectionNameExternalAuths)
	if err != nil {
		return fmt.Errorf("failed to find externalAuths collection: %w", err)
	}

	extBase := types.NowDateTime().Add(-5 * time.Minute)
	extAuths := []struct {
		id         string
		colRef     string
		recordRef  string
		provider   string
		providerId string
		created    types.DateTime
	}{
		{"f1z5b3843pzc964", clients.Id, "gk390qegs4y47wn", "google", "test456", extBase},
		{"dlmflokuq1xl342", usersCol.Id, "4q1xlclmfloku33", "gitlab", "test123", extBase.Add(1 * time.Minute)},
		{"clmflokuq1xl341", usersCol.Id, "4q1xlclmfloku33", "google", "test123", extBase.Add(2 * time.Minute)},
		{"5eto7nmys833164", usersCol.Id, "bgs820n361vj1qd", "github", "test456", extBase.Add(3 * time.Minute)},
	}

	for _, x := range extAuths {
		existing, _ := app.FindRecordById(extAuthsCol, x.id)
		if existing != nil {
			continue
		}
		rec := core.NewRecord(extAuthsCol)
		rec.Id = x.id
		rec.SetRaw("created", x.created)
		rec.Set("collectionRef", x.colRef)
		rec.Set("recordRef", x.recordRef)
		rec.Set("provider", x.provider)
		rec.Set("providerId", x.providerId)
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("failed to create external auth %s: %w", x.id, err)
		}
	}

	// ---------------------------------------------------------------
	// File data (bypass validation)
	// ---------------------------------------------------------------

	demo1Record, _ := app.FindRecordById(demo1, "84nmscqy84lsi1t")
	if demo1Record != nil {
		demo1Record.SetRaw("file_one", "test_d61b33QdDU.txt")
		demo1Record.SetRaw("file_many", `["test_QZFjKjXchk.txt"]`)
		app.SaveNoValidate(demo1Record)
	}

	// reference the committed storage files (tests/data/storage) from the record
	// fields so that the file download scenarios don't 404 on existing files.
	// Using direct SQL to avoid the SaveNoValidate relation reset quirk.
	app.DB().NewQuery(
		`UPDATE "users" SET "avatar" = '300_1SEi6Q6U72.png'
		WHERE "id" = '4q1xlclmfloku33'
		AND ("avatar" IS NULL OR "avatar" = '')`,
	).Execute()
	app.DB().NewQuery(
		`UPDATE "users" SET "avatar" = 'test_kfd2wYLxkz.txt'
		WHERE "id" = 'oap640cot4yru2s'
		AND ("avatar" IS NULL OR "avatar" = '')`,
	).Execute()
	app.DB().NewQuery(
		`UPDATE "demo1" SET "file_one" = '300_Jsjq7RdBgA.png'
		WHERE "id" = 'al1h9ijdeojtsjy'
		AND ("file_one" IS NULL OR "file_one" = '')`,
	).Execute()

	// re-apply the demo1 relation values after the file re-save above
	// (the file save bypasses validation and can reset relation fields)
	setRel(demo1, "84nmscqy84lsi1t", map[string]any{
		"rel_one":  "",
		"rel_many": []string{"oap640cot4yru2s"},
	})
	setRel(demo1, "al1h9ijdeojtsjy", map[string]any{
		"rel_one":  "84nmscqy84lsi1t",
		"rel_many": []string{"bgs820n361vj1qd", "4q1xlclmfloku33", "oap640cot4yru2s"},
	})

	demo3lcl, _ := app.FindRecordById(demo3, "lcl9d87w22ml6jy")
	if demo3lcl != nil {
		demo3lcl.SetRaw("files", `["300_UhLKX91HVb.png","test_FLurQTgrY8.txt"]`)
		app.SaveNoValidate(demo3lcl)
	}

	demo3_7nwo, _ := app.FindRecordById(demo3, "7nwo8tuiatetxdm")
	if demo3_7nwo != nil {
		demo3_7nwo.SetRaw("files", `["test_JnXeKEwgwr.txt"]`)
		app.SaveNoValidate(demo3_7nwo)
	}

	demo3mk5, _ := app.FindRecordById(demo3, "mk5fmymtx4wsprk")
	if demo3mk5 != nil {
		demo3mk5.SetRaw("files", `["300_JdfBOieXAW.png"]`)
		app.SaveNoValidate(demo3mk5)
	}

	// ---------------------------------------------------------------
	// Deterministic _collections.created normalization
	// ---------------------------------------------------------------

	// Collections get their "created" value from time.Now() on save
	// (unconditionally overwritten for new collections by onCollectionSave,
	// so the model path can't be used to preset it). Same-millisecond ties
	// make ORDER BY created non-deterministic and can flip the
	// "?page=2&perPage=2&sort=-created" collections paging test.
	//
	// The raw UPDATE below reassigns strictly increasing timestamps
	// (10ms apart) according to the EXPLICIT creation order below.
	// NB! The array_position tie-break by name list is required because
	// the collection ids don't sort lexicographically in creation order,
	// and sorting by the racy "created" itself could permute the view trio
	// (view1/view2/numeric_id_view are created back-to-back).
	//
	// When adding a new seed collection, append its name to the array
	// (unknown names sort last and are treated as the newest).
	if _, err := app.DB().NewQuery(`
		WITH ordered AS (
			SELECT id, ROW_NUMBER() OVER (
				ORDER BY array_position(ARRAY[
					'_mfas','_otps','_externalAuths','_authOrigins','_superusers','users',
					'demo1','demo2','demo3','nologin','clients','demo4','demo5',
					'view1','view2','numeric_id_view'
				], name) ASC
			) AS rn
			FROM _collections
		)
		UPDATE _collections AS c
		SET created = '2022-01-01 00:00:00.000Z'::timestamptz + (ordered.rn * interval '10 milliseconds')
		FROM ordered
		WHERE c.id = ordered.id
	`).Execute(); err != nil {
		return fmt.Errorf("failed to normalize _collections.created: %w", err)
	}

	// refresh the collection cache with the normalized values
	// (not strictly needed for the cloned test apps since they reload the
	// cache on Bootstrap, but keeps the seeding app state consistent)
	if err := app.ReloadCachedCollections(); err != nil {
		return fmt.Errorf("failed to reload cached collections after normalization: %w", err)
	}

	log.Println("[seed] Test data seeded successfully")
	return nil
}

func stringifyEq(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
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
