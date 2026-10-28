$repo = 'D:\Distributed_Network_system_Hiwi_TUD\tud-dtn-lifecycle-management'
$stateDir = Join-Path $repo 'experiments\store-forward'
$matrixCsv = Join-Path $repo 'experiments\scenarios\results\scenario_results.csv'
$queueLog = Join-Path $stateDir 'queue.log'
$stdout = Join-Path $stateDir 'store-forward.stdout.log'
$stderr = Join-Path $stateDir 'store-forward.stderr.log'
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null

"$(Get-Date -Format o) queued until the baseline CSV contains 90 runs" | Set-Content $queueLog
while ($true) {
    $count = if (Test-Path $matrixCsv) { @(Import-Csv $matrixCsv).Count } else { 0 }
    if ($count -ge 90) { break }
    Start-Sleep -Seconds 60
}
"$(Get-Date -Format o) baseline has $count rows; starting store-forward validation" | Add-Content $queueLog

$arguments = @(
    '--cd', '/mnt/d/Distributed_Network_system_Hiwi_TUD/tud-dtn-lifecycle-management',
    'bash', 'experiments/scripts/run_store_forward_1h.sh', 'flannel', 'leo', '1'
)
$process = Start-Process -FilePath 'wsl.exe' -ArgumentList $arguments `
    -RedirectStandardOutput $stdout -RedirectStandardError $stderr `
    -WindowStyle Hidden -PassThru
"$(Get-Date -Format o) store-forward PID $($process.Id)" | Add-Content $queueLog
$process.WaitForExit()
"$(Get-Date -Format o) store-forward exit code $($process.ExitCode)" | Add-Content $queueLog
