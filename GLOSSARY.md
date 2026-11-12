# iMessage Anywhere

Lets Max use Messages from any of his machines on his LAN or Tailnet by going through the one Mac that is signed in to Messages.

## Language

**Host Mac**:
The one Mac that is signed in to Messages and that every Client goes through; today the work Mac, `work-macbook`.
_Avoid_: server, "my Mac", relay

**Proxy**:
The program on the Host Mac that Clients connect to; it reads and sends Messages on their behalf through Messages.app.
_Avoid_: server, bridge, backend, daemon

**Client**:
A browser on any of Max's machines (GNU+Linux, Windows, Android) that uses the Host Mac's Messages through this project.
_Avoid_: frontend, device, remote

**Conversation**:
One 1:1 or group exchange as Messages.app shows it in its sidebar.
_Avoid_: chat, thread (a Thread is a reply chain inside a Conversation)

**Message**:
One item sent or received in a Conversation, whatever its Service.
_Avoid_: iMessage (that is one Service), text

**Service**:
The network a Message travelled over: iMessage, SMS, or RCS. SMS and RCS reach the Host Mac through the iPhone's Text Message Forwarding.
_Avoid_: protocol, channel, bubble color
