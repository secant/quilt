From quilt/quilt
Maintainer Ethan J. Jackson

RUN apk add --no-cache bash git python curl openssh-client
Copy bin/* /bin/
Copy tests /tests
Copy config/id_rsa /root/.ssh/id_rsa
RUN chmod 0600 /root/.ssh/id_rsa
RUN mkdir /.quilt && chmod -R 777 /.quilt && mkdir /.quilt/config
Copy config/infrastructure.spec /.quilt/config/infrastructure.spec
Entrypoint ["quilt-tester"]
