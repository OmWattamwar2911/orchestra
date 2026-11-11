Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Resilience 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-70"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint7-poster-resilience-1440.png"', '--window-size=1440,1100', '"http://localhost:3000/app/resilience"' -Wait

Write-Host "Capturing Resilience 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-71"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint7-poster-resilience-390.png"', '--window-size=390,1100', '"http://localhost:3000/app/resilience"' -Wait

Write-Host "Capturing Resilience Full 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-72"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint7-poster-resilience-full-1440.png"', '--window-size=1440,2400', '"http://localhost:3000/app/resilience"' -Wait

Copy-Item "$outDir\checkpoint7-poster-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\checkpoint7-poster-*.png" | Format-Table Name, Length
