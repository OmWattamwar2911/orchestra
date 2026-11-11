Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Demo Landing 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-20"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint2-poster-landing-1440.png"', '--window-size=1440,900', '"http://localhost:3000/?demo=true"' -Wait

Write-Host "Capturing Demo Landing 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-21"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint2-poster-landing-390.png"', '--window-size=390,844', '"http://localhost:3000/?demo=true"' -Wait

Write-Host "Capturing Demo App 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-22"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint2-poster-app-1440.png"', '--window-size=1440,900', '"http://localhost:3000/app?demo=true"' -Wait

Write-Host "Capturing Demo App 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-23"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint2-poster-app-390.png"', '--window-size=390,844', '"http://localhost:3000/app?demo=true"' -Wait

Copy-Item "$outDir\checkpoint2-poster-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\checkpoint2-poster-*.png" | Format-Table Name, Length
