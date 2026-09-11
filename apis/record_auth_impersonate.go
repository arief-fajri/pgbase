package apis

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/arief-fajri/pgbase/core"
	validation "github.com/pocketbase/ozzo-validation/v4"
)

// impersonateMaxDurationEnv overrides the upper bound (in seconds) allowed for a
// superuser-issued impersonate token duration. Defaults to [defaultImpersonateMaxDuration]
// (30 days) so a leaked token cannot grant static auth access for an unbounded period.
const impersonateMaxDurationEnv = "PB_IMPERSONATE_MAX_TOKEN_DURATION"

// defaultImpersonateMaxDuration is the default cap (30 days in seconds).
const defaultImpersonateMaxDuration = int64(30 * 24 * 60 * 60)

// maxImpersonateDuration returns the configured cap in seconds.
func maxImpersonateDuration() int64 {
	if raw := strings.TrimSpace(os.Getenv(impersonateMaxDurationEnv)); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			return v
		}
	}
	return defaultImpersonateMaxDuration
}

// note: for now allow superusers but it may change in the future to allow access
// also to users with "Manage API" rule access depending on the use cases that will arise
func recordAuthImpersonate(e *core.RequestEvent) error {
	if !e.HasSuperuserAuth() {
		return e.ForbiddenError("", nil)
	}

	collection, err := findAuthCollection(e)
	if err != nil {
		return err
	}

	record, err := e.App.FindRecordById(collection, e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("", err)
	}

	form := &impersonateForm{}
	if err = e.BindBody(form); err != nil {
		return e.BadRequestError("An error occurred while loading the submitted data.", err)
	}
	if err = form.validate(); err != nil {
		return e.BadRequestError("An error occurred while validating the submitted data.", err)
	}

	token, err := record.NewStaticAuthToken(time.Duration(form.Duration) * time.Second)
	if err != nil {
		return e.InternalServerError("Failed to generate static auth token", err)
	}

	return recordAuthResponse(e, record, token, "", nil)
}

// -------------------------------------------------------------------

type impersonateForm struct {
	// Duration is the optional custom token duration in seconds.
	Duration int64 `form:"duration" json:"duration"`
}

func (form *impersonateForm) validate() error {
	return validation.ValidateStruct(form,
		validation.Field(&form.Duration, validation.Min(0), validation.Max(maxImpersonateDuration())),
	)
}
