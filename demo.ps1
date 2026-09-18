Write-Host "1) Backend nodes" -ForegroundColor Cyan
Invoke-RestMethod http://localhost:8081/api/node | ConvertTo-Json
Invoke-RestMethod http://localhost:8082/api/node | ConvertTo-Json

Write-Host "`n2) Creating booking through backend-1" -ForegroundColor Cyan
$body = @{
  apartment_id = 1
  guest_name = "Demo User"
  email = "demo@example.com"
  check_in = "2026-11-10"
  check_out = "2026-11-12"
  guests = 2
} | ConvertTo-Json

$booking = Invoke-RestMethod -Method Post -Uri http://localhost:8081/api/bookings -ContentType "application/json" -Body $body
$booking | ConvertTo-Json

Write-Host "`n3) Reading the same booking through backend-2" -ForegroundColor Cyan
Invoke-RestMethod ("http://localhost:8082/api/bookings/" + $booking.id) | ConvertTo-Json -Depth 4
