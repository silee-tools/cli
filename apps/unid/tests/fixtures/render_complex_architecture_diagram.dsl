collision off
box 4 2 31 3 id=bb s=r a=c c=Browser (Scenario B)<br>Connect BO User
box 56 2 36 3 id=ba s=r a=c c=Browser (Scenario A)<br>Connect FO User
box 114 2 38 3 id=bc s=r a=c c=Browser (Scenario C)<br>Console Admin
text 22 7 c=B1. as-is: https://app-bo.example.com
text 22 8 c=    to-be: https://app.example.com/backoffice
text 22 10 c=A1. as-is: https://app-fo.example.com
text 22 11 c=    to-be: https://app.example.com
text 22 13 c=C1. as-is: https://console.example.com
text 22 14 c=    to-be: https://console.example.com
box 61 16 30 1 id=foc s=r a=c c=FO Client (Port 3004)
text 79 19 c=A2. as-is: GET https://api.example.com/connect/v1/users
text 79 20 c=    to-be: GET https://app.example.com/api/v1/users
box 4 22 31 1 id=boc s=r a=c c=BO Client (Port 3104)
box 114 22 38 1 id=csc s=r a=c c=Console Client (MF Host, Port 5001)
text 22 25 c=B2. as-is: GET https://api.example.com/connect/v1/back-office/users
text 22 26 c=    to-be: GET https://app.example.com/api/backoffice/v1/users
box 30 28 92 18 id=cs s=d lp=t lg=App Server (Port 8007) c=Endpoints:<br>  FO:      /v1/users, /v1/orders, /v1/organizations<br>  BO:      /v1/back-office/users, /v1/back-office/orders<br>  Console: /api/v1/console/{users,orgs,roles}<br>  MF:      /mf/**<br><br>Auth:<br>  @Order(1) /api/v*/console/**<br>    -> ConsoleApiAuthFilter<br>  @Order(2) /**<br>    -> permitAll + RoleInterceptor
box 70 40 48 3 id=ccc s=h a=c c=Console MF Client<br>(MF Remote)<br>Served as static /mf/**
box 36 52 84 5 id=css s=d lp=t lg=Console Server (Port 8504) c=Auth:     /console/v1/sessions/verify -> session validation<br>Employee: /v1/employees/me (self only)<br>Employee: /v1/employees (CUD, admin menu)
arrow ba.b foc.t
arrow foc.b cs.t
arrow bb.b boc.t
arrow boc.b cs.l
arrow bc.b csc.t
arrow csc.b css.r pos=l lg=C2
arrow csc.l ccc.r pos=t lg=C3 (MF)
arrow cs.b css.t pos=r lg=C5
text 2 50 c=C2. as-is: https://console.example.com  |  to-be: https://console.example.com (session mgmt)
text 2 60 c=C5. to-be: GET https://api.example.com/console/v1/sessions/verify (mTLS via sidecar)
