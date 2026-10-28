FROM php:8.3-apache

ENV RESULTS_CSV=/data/persistent_store_forward_results.csv
ENV APP_SOURCE_ROOT=/var/www/src
COPY public/ /var/www/html/
COPY src/ /var/www/src/
RUN chown -R www-data:www-data /var/www/html /var/www/src
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD curl -fsS http://localhost/health.php || exit 1
