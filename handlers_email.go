package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"
)

const challengeCookie = "school_nanny_challenge"

const passwordMismatch = "Those passwords did not match."

func (a *App) sendConfirmMail(user *User) error {
	if a.mailer == nil || user == nil {
		return nil
	}
	raw, err := randomToken()
	if err != nil {
		return err
	}
	if err := a.control.issueEmailToken(user.ID, tokenConfirm, raw, "", confirmTokenTTL); err != nil {
		return err
	}
	body := "Confirm your School Nanny account:\n\n" + linkURL(a.baseURL, "/verify-email", raw) + "\n"
	return a.sendMail(user.Email, "Confirm your School Nanny account", body)
}

func (a *App) issueChallengeCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     challengeCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(signinCodeTTL.Seconds()),
	})
}

func (a *App) clearChallengeCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     challengeCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (a *App) beginEmailSignin(w http.ResponseWriter, r *http.Request, user *User) {
	if a.mailer == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "Email is not configured.",
		})
		return
	}
	code, err := randomSigninCode()
	if err != nil {
		a.serverError(w, err)
		return
	}
	challenge, err := randomToken()
	if err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.control.issueEmailToken(user.ID, tokenSignin, code, challenge, signinCodeTTL); err != nil {
		a.serverError(w, err)
		return
	}
	body := "Your School Nanny sign-in code is " + code + "\n\nIt expires in 10 minutes.\n"
	if err := a.sendMail(user.Email, "Your School Nanny sign-in code", body); err != nil {
		log.Printf("sign-in code mail: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "Email is not configured.",
		})
		return
	}
	a.issueChallengeCookie(w, challenge)
	a.render(w, "login", map[string]any{
		"Active":   "login",
		"Hosted":   true,
		"NeedCode": true,
	})
}

func (a *App) handleLoginCodeForm(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	a.render(w, "login", map[string]any{
		"Active":   "login",
		"Hosted":   true,
		"NeedCode": true,
	})
}

func (a *App) handleLoginCode(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	cookie, err := r.Cookie(challengeCookie)
	if err != nil || cookie.Value == "" {
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "That code did not match.",
		})
		return
	}
	row, err := a.control.findSigninChallenge(cookie.Value)
	if err != nil {
		a.clearChallengeCookie(w)
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "That code did not match.",
		})
		return
	}
	if !signinCodeMatches(row, r.FormValue("code")) {
		burned, noteErr := a.control.noteSigninFailure(row)
		if noteErr != nil {
			a.serverError(w, noteErr)
			return
		}
		if burned {
			a.clearChallengeCookie(w)
		}
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "login", map[string]any{
			"Active":   "login",
			"Hosted":   true,
			"NeedCode": !burned,
			"Error":    "That code did not match.",
		})
		return
	}
	if err := a.control.consumeEmailToken(row.id); err != nil {
		a.serverError(w, err)
		return
	}
	familyID, _, err := a.control.ownerFamily(row.account)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.clearChallengeCookie(w)
	sess, err := a.control.CreateSession(row.account, familyID, sessionLifetime)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.issueHostedSession(w, sess)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	row, err := a.control.findEmailToken(tokenConfirm, r.URL.Query().Get("token"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "That link did not work.",
		})
		return
	}
	if err := a.control.consumeEmailToken(row.id); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.control.markEmailVerified(row.account); err != nil {
		a.serverError(w, err)
		return
	}
	familyID, _, err := a.control.ownerFamily(row.account)
	if err != nil {
		a.serverError(w, err)
		return
	}
	sess, err := a.control.CreateSession(row.account, familyID, sessionLifetime)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.issueHostedSession(w, sess)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleResendConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if !a.requireOwner(w, r) {
		return
	}
	sess := sessionFrom(r)
	if sess == nil {
		http.Error(w, "Only the family owner can do that.", http.StatusForbidden)
		return
	}
	if a.mailer == nil {
		http.Error(w, "Email is not configured.", http.StatusServiceUnavailable)
		return
	}
	if created, ok := a.control.recentConfirmToken(sess.AccountID); ok && time.Since(created) < resendMinGap {
		http.Error(w, "Wait a minute before sending another link.", http.StatusTooManyRequests)
		return
	}
	_, email, err := a.control.ownerFamily(sess.AccountID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	user := &User{ID: sess.AccountID, Email: email}
	if err := a.sendConfirmMail(user); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/?confirm=sent")
}

func (a *App) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	msg := "If that email is registered, we sent a reset link."
	if a.mailer == nil {
		msg = "Email is not configured."
	} else if id, _, err := a.control.ownerByEmail(r.FormValue("email")); err == nil {
		raw, tokErr := randomToken()
		if tokErr != nil {
			a.serverError(w, tokErr)
			return
		}
		if err := a.control.issueEmailToken(id, tokenReset, raw, "", resetTokenTTL); err != nil {
			a.serverError(w, err)
			return
		}
		_, email, famErr := a.control.ownerFamily(id)
		if famErr != nil {
			a.serverError(w, famErr)
			return
		}
		body := "Reset your School Nanny password:\n\n" + linkURL(a.baseURL, "/reset-password", raw) + "\n\nThis link expires in one hour.\n"
		if err := a.sendMail(email, "Reset your School Nanny password", body); err != nil {
			log.Printf("reset mail: %v", err)
			msg = "Email is not configured."
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, err)
		return
	}
	a.render(w, "login", map[string]any{
		"Active": "login",
		"Hosted": true,
		"Notice": msg,
	})
}

func (a *App) handleResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	token := r.URL.Query().Get("token")
	if _, err := a.control.findEmailToken(tokenReset, token); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "That link did not work.",
		})
		return
	}
	a.render(w, "reset_password", map[string]any{
		"Active": "login",
		"Hosted": true,
		"Token":  token,
	})
}

func (a *App) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	row, err := a.control.findEmailToken(tokenReset, token)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		a.render(w, "login", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Error":  "That link did not work.",
		})
		return
	}
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")
	fail := func(msg string) {
		_ = a.control.noteResetFailure(row.id, row.fails)
		w.WriteHeader(http.StatusBadRequest)
		a.render(w, "reset_password", map[string]any{
			"Active": "login",
			"Hosted": true,
			"Token":  token,
			"Error":  msg,
		})
	}
	if !passwordsMatch(password, confirm) {
		fail(passwordMismatch)
		return
	}
	if err := a.control.UpdatePassword(row.account, password); err != nil {
		fail(err.Error())
		return
	}
	if err := a.control.consumeEmailToken(row.id); err != nil {
		a.serverError(w, err)
		return
	}
	_ = a.control.markEmailVerified(row.account)
	_ = a.control.KillSessionsForAccount(row.account)
	familyID, _, err := a.control.ownerFamily(row.account)
	if err != nil {
		a.serverError(w, err)
		return
	}
	sess, err := a.control.CreateSession(row.account, familyID, sessionLifetime)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.issueHostedSession(w, sess)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleSaveEmail2FA(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if !a.requireOwner(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	sess := sessionFrom(r)
	if err := a.control.setEmail2FA(sess.AccountID, r.FormValue("email_2fa") == "1"); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/access?saved=email-2fa")
}
