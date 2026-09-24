from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = "/tmp/nd_fake_dlna.log"
STATE = {"transport": "PLAYING", "rel": "0:00:07", "duration": "0:03:20", "volume": "61"}

ROOT_DESC = """<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
    <friendlyName>Fake Xiaoai</friendlyName>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
        <controlURL>/upnp/control/AVTransport</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
        <controlURL>/upnp/control/RenderingControl</controlURL>
      </service>
    </serviceList>
  </device>
</root>"""

def log(msg):
    with open(LOG, "a") as f:
        f.write(msg + "\n")

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        log("GET " + self.path)
        if self.path == "/rootDesc.xml":
            body = ROOT_DESC.encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/xml")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length).decode()
        soap_action = self.headers.get("SOAPAction", "")
        action = soap_action.split("#")[-1].strip('"')
        log(f"POST {self.path} action={action}")
        if action in ("SetAVTransportURI", "Seek"):
            log("BODY " + body)
        if action == "GetTransportInfo":
            inner = f"<CurrentTransportState>{STATE['transport']}</CurrentTransportState>"
        elif action == "GetPositionInfo":
            inner = f"<TrackDuration>{STATE['duration']}</TrackDuration><RelTime>{STATE['rel']}</RelTime>"
        elif action == "GetVolume":
            inner = f"<CurrentVolume>{STATE['volume']}</CurrentVolume>"
        else:
            inner = ""
        if action == "Stop":
            STATE["transport"] = "STOPPED"
        if action == "Pause":
            STATE["transport"] = "PAUSED_PLAYBACK"
        if action == "Play":
            STATE["transport"] = "PLAYING"
        resp = (
            '<?xml version="1.0"?>'
            '<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>'
            f'<u:{action}Response xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">{inner}'
            f'</u:{action}Response></s:Body></s:Envelope>'
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/xml")
        self.send_header("Content-Length", str(len(resp)))
        self.end_headers()
        self.wfile.write(resp)

HTTPServer(("127.0.0.1", 1800), Handler).serve_forever()
