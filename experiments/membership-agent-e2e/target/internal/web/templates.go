package web

const pageTemplates = `
{{define "list"}}<!doctype html>
<html><head><meta charset="utf-8"><title>Users</title></head><body>
<main><h1>Users</h1>
{{if .Failure}}<div role="alert" data-state="failure">{{.Failure}}</div>{{else}}
<form method="get" action="/admin/users">
  <label>Search <input name="q" value="{{.Search}}"></label>
  <label>Team <select name="team"><option value="">All</option>{{range .Teams}}<option value="{{.ID}}" {{if eq $.Team .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>
  <label>Plan <select name="plan"><option value="">All</option>{{range .Plans}}<option value="{{.}}" {{if eq $.Plan .}}selected{{end}}>{{.}}</option>{{end}}</select></label>
  <label>Status <select name="status"><option value="">All</option>{{range .Statuses}}<option value="{{.}}" {{if eq $.Status .}}selected{{end}}>{{.}}</option>{{end}}</select></label>
  <button type="submit">Apply</button>
</form>
{{if .Empty}}<p data-state="empty">No users found.</p>{{else}}
<table><thead><tr><th>Name</th><th>Nickname</th><th>Email</th><th>Team</th><th>Plan</th><th>Status</th><th>Actions</th></tr></thead><tbody>
{{range .Users}}<tr data-user-row="{{.ID}}"><td>{{.Name}}</td><td>{{.Nickname}}</td><td>{{.Email}}</td><td>{{.TeamName}}</td><td>{{.Plan}}</td><td>{{.Status}}</td><td><a href="/admin/users/{{.ID}}">View</a> <a href="/admin/users/{{.ID}}/edit">Edit</a></td></tr>{{end}}
</tbody></table>
{{end}}
<nav>{{if .Previous}}<a rel="prev" href="{{.Previous}}">Previous</a>{{end}} {{if .Next}}<a rel="next" href="{{.Next}}">Next</a>{{end}}</nav>
{{end}}</main></body></html>{{end}}

{{define "detail"}}<!doctype html>
<html><head><meta charset="utf-8"><title>User detail</title></head><body>
<main><h1>User detail</h1>
{{if .Failure}}<div role="alert" data-state="failure">{{.Failure}}</div>{{else if .Empty}}<p data-state="empty">User not found.</p>{{else}}
<dl><dt>Name</dt><dd>{{.User.Name}}</dd><dt>Nickname</dt><dd>{{.User.Nickname}}</dd><dt>Email</dt><dd>{{.User.Email}}</dd><dt>Team</dt><dd>{{.User.TeamName}}</dd><dt>Plan</dt><dd>{{.User.Plan}}</dd><dt>Status</dt><dd>{{.User.Status}}</dd></dl>
<a href="/admin/users/{{.User.ID}}/edit">Edit</a>
{{end}}</main></body></html>{{end}}

{{define "edit"}}<!doctype html>
<html><head><meta charset="utf-8"><title>Edit user</title></head><body>
<main><h1>Edit user</h1>
{{if .Failure}}<div role="alert" data-state="failure">{{.Failure}}</div>{{end}}
{{with index .Errors "submission"}}<p role="alert" data-error="submission">{{.}}</p>{{end}}
{{if .Token}}<form method="post" action="/admin/users/{{.UserID}}/edit">
  <input type="hidden" name="submission" value="{{.Token}}">
  <label>Name <input name="name" value="{{.Form.Name}}"></label>{{with index .Errors "name"}}<p role="alert" data-error="name">{{.}}</p>{{end}}
  <label>Nickname <input name="nickname" value="{{.Form.Nickname}}"></label>
  <label>Email <input name="email" value="{{.Form.Email}}"></label>{{with index .Errors "email"}}<p role="alert" data-error="email">{{.}}</p>{{end}}
  <label>Team <select name="team"><option value="">None</option>{{range .Teams}}<option value="{{.ID}}" {{if eq $.Form.TeamID .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>{{with index .Errors "team"}}<p role="alert" data-error="team">{{.}}</p>{{end}}
  <label>Plan <select name="plan"><option value="">Select</option>{{range .Plans}}<option value="{{.}}" {{if eq $.Form.Plan .}}selected{{end}}>{{.}}</option>{{end}}</select></label>{{with index .Errors "plan"}}<p role="alert" data-error="plan">{{.}}</p>{{end}}
  <button type="submit">Save</button>
</form>{{end}}
</main></body></html>{{end}}
{{define "signup"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Create your account</title></head><body><main>
<h1>Create your account</h1>
{{with index .Errors "form"}}<p role="alert" data-error="form">{{.}}</p>{{end}}
<form method="post" action="/members/signup">
  <input type="hidden" name="submission" value="{{.Token}}">
  <label>Name <input name="name" value="{{.Name}}" required></label>{{with index .Errors "name"}}<p role="alert" data-error="name">{{.}}</p>{{end}}
  <label>Email <input name="email" value="{{.Email}}" required></label>{{with index .Errors "email"}}<p role="alert" data-error="email">{{.}}</p>{{end}}
  <label>Password <input name="password" type="password" required></label>{{with index .Errors "password"}}<p role="alert" data-error="password">{{.}}</p>{{end}}
  <button type="submit">Create account</button>
</form>
<p><a href="/members/signin">Already a member? Sign in</a></p>
</main></body></html>{{end}}

{{define "check-email"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Check your email</title></head><body><main>
<h1>Check your email</h1>
{{with .Notice}}<p data-notice="resend">{{.}}</p>{{end}}
{{with index .Errors "form"}}<p role="alert" data-error="form">{{.}}</p>{{end}}
<form method="post" action="/members/check-email">
  <input type="hidden" name="submission" value="{{.Token}}">
  <label>Email <input name="email" value="{{.Email}}" required></label>
  <button type="submit">Send a new link</button>
</form>
</main></body></html>{{end}}

{{define "registered"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Registration complete</title></head><body><main>
<h1>Registration complete</h1>
<p data-notice="registered">{{.Notice}}</p>
<p><a href="/members/signin">Continue to sign in</a></p>
</main></body></html>{{end}}

{{define "signin"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Sign in</title></head><body><main>
<h1>Sign in</h1>
{{with index .Errors "form"}}<p role="alert" data-error="form">{{.}}</p>{{end}}
<form method="post" action="/members/signin">
  <input type="hidden" name="submission" value="{{.Token}}">
  <label>Email <input name="email" value="{{.Email}}" required></label>
  <label>Password <input name="password" type="password" required></label>
  <button type="submit">Sign in</button>
</form>
</main></body></html>{{end}}

{{define "profile"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Your profile</title></head><body><main>
<h1>Your profile</h1>
{{if .Empty}}<p data-state="empty">Your profile is no longer available.</p>{{else}}
{{with index .Errors "form"}}<p role="alert" data-state="failure">{{.}}</p>{{end}}
{{if not .Errors}}<dl>
  <dt>Name</dt><dd data-field="name">{{.Name}}</dd>
  <dt>Nickname</dt><dd data-field="nickname">{{.Nickname}}</dd>
  <dt>Email</dt><dd data-field="email">{{.Email}}</dd>
  <dt>Status</dt><dd data-field="status">{{.Status}}</dd>
</dl>
<p><a href="/members/users/{{.ID}}/edit">Edit profile</a></p>{{end}}{{end}}
<form method="post" action="/members/signout"><button type="submit">Sign out</button></form>
</main></body></html>{{end}}

{{define "profile-edit"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Edit your profile</title></head><body><main>
<h1>Edit your profile</h1>
{{with index .Errors "form"}}<p role="alert" data-error="form">{{.}}</p>{{end}}
<form method="post" action="/members/users/{{.ID}}/edit">
  <input type="hidden" name="submission" value="{{.Token}}">
  <label>Name <input name="name" value="{{.Name}}" required></label>{{with index .Errors "name"}}<p role="alert" data-error="name">{{.}}</p>{{end}}
  <label>Nickname <input name="nickname" value="{{.Nickname}}"></label>
  <button type="submit">Save</button>
</form>
<p><a href="/members/users/{{.ID}}">Cancel</a></p>
</main></body></html>{{end}}
`
