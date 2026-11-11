Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\demo-recording"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

if (!(Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir -Force | Out-Null
}

Write-Host "=== RECORDING SCENE 1/5: LANDING HERO & SCORE VIEW ==="
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-demo-1"', "--screenshot=`"$outDir\demo-scene1-landing.png`"", '--window-size=1440,900', '"http://localhost:3000/"' -Wait

Write-Host "=== RECORDING SCENE 2/5: CLUSTER OVERVIEW ==="
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-demo-2"', "--screenshot=`"$outDir\demo-scene2-cluster.png`"", '--window-size=1440,900', '"http://localhost:3000/app"' -Wait

Write-Host "=== RECORDING SCENE 3/5: SUBMIT DIAMOND DAG ==="
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-demo-3"', "--screenshot=`"$outDir\demo-scene3-submit.png`"", '--window-size=1440,900', '"http://localhost:3000/app/submit"' -Wait

Write-Host "=== RECORDING SCENE 4/5: WORKFLOWS & DAG DETAIL ==="
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-demo-4"', "--screenshot=`"$outDir\demo-scene4-workflows.png`"", '--window-size=1440,900', '"http://localhost:3000/app/workflows"' -Wait

Write-Host "=== RECORDING SCENE 5/5: RESILIENCE LAB & LEADER KILL ==="
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-demo-5"', "--screenshot=`"$outDir\demo-scene5-resilience.png`"", '--window-size=1440,900', '"http://localhost:3000/app/resilience"' -Wait

Copy-Item "$outDir\demo-scene*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\demo-scene*.png" | Format-Table Name, Length
Write-Host "Demo recording sequence complete!"
