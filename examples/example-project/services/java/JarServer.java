import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;

/**
 * Entry point for the jar-packaged demo service.
 *
 * prepare.ps1 packages this class into java-jar-api.jar with a Main-Class manifest
 * entry, so the service is deployed as a single jar artifact and started with
 * `java -jar java-jar-api.jar <port> <name>`. The extra "packaging" field in the
 * health payload makes it obvious which delivery shape is running.
 */
public class JarServer {
  public static void main(String[] args) throws Exception {
    int port = Integer.parseInt(args[0]);
    String name = args[1];
    HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", port), 0);
    server.createContext("/healthz", exchange -> reply(exchange, name));
    server.createContext("/", exchange -> reply(exchange, name));
    server.start();
  }

  static void reply(HttpExchange exchange, String name) throws IOException {
    byte[] body = ("{\"service\":\"" + name + "\",\"status\":\"ok\",\"packaging\":\"jar\"}")
        .getBytes(StandardCharsets.UTF_8);
    exchange.getResponseHeaders().set("Content-Type", "application/json");
    exchange.sendResponseHeaders(200, body.length);
    exchange.getResponseBody().write(body);
    exchange.close();
  }
}
