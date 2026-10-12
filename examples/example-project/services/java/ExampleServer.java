import com.sun.net.httpserver.HttpServer;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;

public class ExampleServer {
  public static void main(String[] args) throws Exception {
    int port = Integer.parseInt(args[0]);
    String name = args[1];
    HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", port), 0);
    server.createContext("/healthz", exchange -> reply(exchange, name));
    server.createContext("/", exchange -> reply(exchange, name));
    server.start();
  }
  static void reply(com.sun.net.httpserver.HttpExchange exchange, String name) throws java.io.IOException {
    byte[] body = ("{\"service\":\"" + name + "\",\"status\":\"ok\"}").getBytes(StandardCharsets.UTF_8);
    exchange.getResponseHeaders().set("Content-Type", "application/json");
    exchange.sendResponseHeaders(200, body.length);
    exchange.getResponseBody().write(body); exchange.close();
  }
}
