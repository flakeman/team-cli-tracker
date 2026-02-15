$ErrorActionPreference = "Stop"

$Base = "http://srv1.abuztech.ru:4101"
$Project = "OPS"

function ApiPost($path, $token, $bodyObj) {
  $json = $bodyObj | ConvertTo-Json -Compress
  return curl.exe -s -X POST "$Base$path" `
    -H "Authorization: Bearer $token" `
    -H "Content-Type: application/json" `
    -d $json
}

function Sleep-Step {
  Start-Sleep -Seconds (Get-Random -Minimum 2 -Maximum 4)
}

Write-Host "== Create 10 issues =="
1..10 | ForEach-Object {
  $id = "OPS-99$_"
  $assignee = if ($_ % 2 -eq 0) { "dev1" } else { "qa1" }
  ApiPost "/api/v1/issue/create" "lead-token" @{
    project_id = $Project
    issue_id   = $id
    summary    = "Smoke task $id"
    assignee   = $assignee
  } | Out-Host
  Sleep-Step
}

Write-Host "== Move tasks with 2-3 sec cadence =="
$movePlan = @(
  @{ id = "OPS-991"; to = "in_progress"; token = "lead-token" },
  @{ id = "OPS-992"; to = "in_progress"; token = "lead-token" },
  @{ id = "OPS-993"; to = "in_progress"; token = "lead-token" },
  @{ id = "OPS-994"; to = "in_progress"; token = "lead-token" },
  @{ id = "OPS-992"; to = "code_review"; token = "dev-token"  },
  @{ id = "OPS-993"; to = "code_review"; token = "dev-token"  },
  @{ id = "OPS-994"; to = "testing";     token = "qa-token"   },
  @{ id = "OPS-991"; to = "done";        token = "lead-token" }
)

$movePlan | ForEach-Object {
  ApiPost "/api/v1/issue/transition" $_.token @{
    project_id = $Project
    issue_id   = $_.id
    to         = $_.to
  } | Out-Host
  Sleep-Step
}

Write-Host "== Add comments =="
@("OPS-991","OPS-995","OPS-999") | ForEach-Object {
  ApiPost "/api/v1/issue/comment" "qa-token" @{
    project_id = $Project
    issue_id   = $_
    text       = "qa-check for $_"
  } | Out-Host
  Sleep-Step
}

Write-Host "== Attach files to OPS-991 and OPS-992 =="
@("OPS-991","OPS-992") | ForEach-Object {
  $issue = $_
  $file = "$issue.txt"
  "file-content-$issue" | Out-File -Encoding ascii $file

  $size = (Get-Item $file).Length
  $initRaw = ApiPost "/api/v1/issue/attachment/initiate" "admin-token" @{
    project_id   = $Project
    issue_id     = $issue
    filename     = $file
    content_type = "text/plain"
    size_bytes   = $size
    title        = "Attachment $issue"
  }
  $init = $initRaw | ConvertFrom-Json

  curl.exe -s -X PUT --data-binary "@$file" $init.upload_url | Out-Null

  $sha = (Get-FileHash ".\$file" -Algorithm SHA256).Hash.ToLower()
  ApiPost "/api/v1/issue/attachment/complete" "admin-token" @{
    project_id      = $Project
    issue_id        = $issue
    attachment_id   = $init.attachment_id
    filename        = $file
    content_type    = "text/plain"
    checksum_sha256 = $sha
  } | Out-Host

  ApiPost "/api/v1/issue/attachment/verify" "admin-token" @{
    project_id    = $Project
    issue_id      = $issue
    attachment_id = $init.attachment_id
  } | Out-Host

  Sleep-Step
}

Write-Host "== Verify role restrictions (viewer should fail) =="
$viewerCreate = ApiPost "/api/v1/issue/create" "viewer-token" @{
  project_id = $Project
  issue_id   = "OPS-9X1"
  summary    = "viewer must fail"
  assignee   = "viewer1"
}
$viewerComment = ApiPost "/api/v1/issue/comment" "viewer-token" @{
  project_id = $Project
  issue_id   = "OPS-991"
  text       = "viewer comment fail"
}
Write-Host $viewerCreate
Write-Host $viewerComment

Write-Host "== Done =="
