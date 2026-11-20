FROM python:3.12-slim
WORKDIR /opt/lab
COPY fake_c2.py phish_panel.py ./
USER 65532:65532
EXPOSE 8080
CMD ["python", "fake_c2.py", "--host", "0.0.0.0"]
