package notify

// layout is the HTML of every email: tables and inline styles, which every
// mail client renders, in the app's light theme. No images or web fonts, so
// nothing is fetched from outside and nothing is blocked.
const layout = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light only"><title>{{.Subject}}</title></head>
<body style="margin:0;padding:0;background:#f2f4f8;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#131a26;">
<span style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Preheader}}</span>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f2f4f8;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border:1px solid #e2e6ec;border-radius:10px;overflow:hidden;">
  <tr><td style="background:#1f4fae;padding:18px 28px;">
    <table role="presentation" cellpadding="0" cellspacing="0"><tr>
      <td style="width:30px;height:30px;background:#8fb3f7;border-radius:7px;"></td>
      <td style="padding-left:12px;color:#ffffff;">
        <div style="font-size:16px;font-weight:600;letter-spacing:.2px;">DF-Atlas</div>
        <div style="font-size:12px;color:#d6e2fb;">Dialog Finance · Network and IPAM</div>
      </td>
    </tr></table>
  </td></tr>
  <tr><td style="padding:28px 28px 8px;">
    <h1 style="margin:0 0 8px;font-size:20px;line-height:1.3;font-weight:600;color:#131a26;">{{.Heading}}</h1>
    {{if .Intro}}<p style="margin:0;font-size:14px;line-height:1.55;color:#4d586a;">{{.Intro}}</p>{{end}}
  </td></tr>
  {{range .Sections}}
  <tr><td style="padding:18px 28px 4px;">
    <div style="font-size:12px;font-weight:600;letter-spacing:.6px;text-transform:uppercase;color:#6f7a8c;margin-bottom:6px;">{{.Title}}</div>
    {{if .Note}}<div style="font-size:13px;color:#4d586a;margin-bottom:8px;">{{.Note}}</div>{{end}}
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid #e2e6ec;border-radius:8px;">
      {{range $i, $r := .Rows}}
      <tr><td style="padding:10px 14px;{{if $i}}border-top:1px solid #e2e6ec;{{end}}font-size:14px;line-height:1.45;">
        {{if $r.Chip}}<span style="display:inline-block;padding:1px 8px;margin-right:8px;border-radius:999px;font-size:11px;font-weight:600;color:{{fg $r.Tone}};background:{{bg $r.Tone}};">{{$r.Chip}}</span>{{end}}
        {{if $r.Path}}<a href="{{url $.Base $r.Path}}" style="color:#131a26;text-decoration:none;">{{$r.Text}}</a>{{else}}{{$r.Text}}{{end}}
        {{if $r.Meta}}<div style="font-size:12px;color:#6f7a8c;margin-top:2px;">{{$r.Meta}}</div>{{end}}
      </td></tr>
      {{end}}
      {{if .More}}<tr><td style="padding:10px 14px;border-top:1px solid #e2e6ec;font-size:13px;color:#6f7a8c;">and {{.More}} more in DF-Atlas</td></tr>{{end}}
    </table>
  </td></tr>
  {{end}}
  {{if .Action.Label}}
  <tr><td style="padding:24px 28px 28px;">
    <a href="{{url .Base .Action.Path}}" style="display:inline-block;background:#2257c9;color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;padding:10px 18px;border-radius:6px;">{{.Action.Label}}</a>
  </td></tr>
  {{end}}
  <tr><td style="padding:16px 28px;background:#f8f9fb;border-top:1px solid #e2e6ec;font-size:12px;line-height:1.5;color:#6f7a8c;">
    {{.Why}} Choose which emails you get in DF-Atlas under <strong>Alerts › Email preferences</strong>.<br>
    DF-Atlas only reads the network. This message was sent automatically; replies aren't read.
  </td></tr>
</table>
</td></tr></table>
</body></html>`
