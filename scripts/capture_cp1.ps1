Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Landing 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-10"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint1-poster-landing-1440.png"', '--window-size=1440,900', '"http://localhost:3000/"' -Wait

Write-Host "Capturing Landing 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-11"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint1-poster-landing-390.png"', '--window-size=390,844', '"http://localhost:3000/"' -Wait

Write-Host "Capturing App 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-12"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint1-poster-app-1440.png"', '--window-size=1440,900', '"http://localhost:3000/app"' -Wait

Write-Host "Capturing App 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-13"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint1-poster-app-390.png"', '--window-size=390,844', '"http://localhost:3000/app"' -Wait

Copy-Item "$outDir\checkpoint1-poster-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\checkpoint1-poster-*.png" | Format-Table Name, Length
