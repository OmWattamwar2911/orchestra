Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Landing 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-30"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint3-poster-landing-1440.png"', '--window-size=1440,900', '"http://localhost:3000/"' -Wait

Write-Host "Capturing Landing 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-31"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint3-poster-landing-390.png"', '--window-size=390,844', '"http://localhost:3000/"' -Wait

Write-Host "Capturing Landing Full 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-32"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint3-poster-landing-full-1440.png"', '--window-size=1440,3200', '"http://localhost:3000/"' -Wait

Copy-Item "$outDir\checkpoint3-poster-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\checkpoint3-poster-*.png" | Format-Table Name, Length
